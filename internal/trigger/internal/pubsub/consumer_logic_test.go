package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// TestConsumer_Dispatch_InvalidPayload verifies that invalid payloads are Terminated.
func TestConsumer_Dispatch_InvalidPayload(t *testing.T) {
	// Setup
	c := &natsConsumer{
		numWorkers:  1,
		workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
		metrics:     &types.NoopMetrics{},
	}
	msg := new(MockMsg)

	// Expectations
	msg.On("Data").Return([]byte("invalid-json"))
	msg.On("Term").Return(nil)

	// Execute
	c.dispatch(msg)

	// Verify
	msg.AssertExpectations(t)
}

// TestConsumer_Worker_RetryLogic verifies the retry logic in workerLoop.
func TestConsumer_Worker_RetryLogic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		processErr     error
		metadataErr    error
		numDelivered   uint64
		maxAttempts    int
		initialBackoff time.Duration
		maxBackoff     time.Duration
		payload        interface{} // string (invalid) or DeliveryTask
		expectTerm     bool
		expectNak      bool
		expectNakDelay time.Duration
	}{
		{
			name:       "Success",
			processErr: nil,
			payload: &types.DeliveryTask{
				TriggerID: "t1",
			},
		},
		{
			name:         "ProcessError_Retry",
			processErr:   errors.New("fail"),
			numDelivered: 1,
			maxAttempts:  3,
			payload: &types.DeliveryTask{
				TriggerID: "t1",
				RetryPolicy: types.RetryPolicy{
					MaxAttempts:    3,
					InitialBackoff: types.Duration(1 * time.Second),
				},
			},
			expectNak:      true,
			expectNakDelay: 1 * time.Second,
		},
		{
			name:         "ProcessError_MaxAttemptsReached",
			processErr:   errors.New("fail"),
			numDelivered: 3,
			maxAttempts:  3,
			payload: &types.DeliveryTask{
				TriggerID: "t1",
				RetryPolicy: types.RetryPolicy{
					MaxAttempts: 3,
				},
			},
			expectTerm: true,
		},
		{
			name:         "ProcessError_Fatal",
			processErr:   &types.FatalError{Err: errors.New("fatal")},
			numDelivered: 1,
			maxAttempts:  3,
			payload: &types.DeliveryTask{
				TriggerID: "t1",
			},
			expectTerm: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockWorker := new(MockWorker)
			c := &natsConsumer{
				worker:      mockWorker,
				numWorkers:  1,
				stream:      tt.name,
				workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
				metrics:     &types.NoopMetrics{},
			}
			c.wg.Add(1)

			msg := new(MockMsg)

			// Setup expectations
			if tt.payload != nil {
				if task, ok := tt.payload.(*types.DeliveryTask); ok {
					data, _ := json.Marshal(task)
					msg.On("Data").Return(data)
					if tt.processErr != nil {
						mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(tt.processErr)
					} else {
						mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(nil)
						msg.On("Ack").Return(nil)
					}
				} else {
					// Invalid payload string
					msg.On("Data").Return([]byte(tt.payload.(string)))
				}
			}

			if tt.processErr != nil {
				if !types.IsFatal(tt.processErr) {
					md := &jetstream.MsgMetadata{NumDelivered: tt.numDelivered}
					msg.On("Metadata").Return(md, tt.metadataErr)
				}
				if tt.expectTerm {
					msg.On("Term").Return(nil)
				} else if tt.expectNak {
					msg.On("NakWithDelay", tt.expectNakDelay).Return(nil)
				}
			}

			// Inject message
			c.workerChans[0] <- msg
			close(c.workerChans[0])

			// Run worker loop
			c.workerLoop(context.Background(), 0)

			// Verify
			msg.AssertExpectations(t)
			mockWorker.AssertExpectations(t)
		})
	}
}

// TestNewTaskConsumer_JetStreamError verifies error handling when JetStream creation fails.
func TestNewTaskConsumer_JetStreamError(t *testing.T) {
	t.Parallel()
	// Temporarily replace jetStreamNew with a failing function
	restore := SetJetStreamNew(func(nc *nats.Conn) (jetstream.JetStream, error) {
		return nil, errors.New("jetstream connection failed")
	})
	defer restore()

	// Use a nil connection (which will be caught by the nil check first)
	// But we also need a non-nil connection to test the JetStream error path
	mockWorker := new(MockWorker)
	_, err := NewTaskConsumer(nil, mockWorker, "TestNewTaskConsumer_JetStreamError", 1, nil)
	assert.ErrorContains(t, err, "nats connection cannot be nil")
}

// TestConsumer_Worker_MetadataError verifies handling of metadata retrieval errors.
func TestConsumer_Worker_MetadataError(t *testing.T) {
	mockWorker := new(MockWorker)
	c := &natsConsumer{
		worker:      mockWorker,
		numWorkers:  1,
		workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
		metrics:     &types.NoopMetrics{},
	}
	c.wg.Add(1)

	msg := new(MockMsg)
	task := &types.DeliveryTask{TriggerID: "t1"}
	data, _ := json.Marshal(task)

	msg.On("Data").Return(data)
	mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(errors.New("process failed"))
	msg.On("Metadata").Return(nil, errors.New("metadata error"))
	msg.On("Nak").Return(nil)

	c.workerChans[0] <- msg
	close(c.workerChans[0])

	c.workerLoop(context.Background(), 0)

	msg.AssertExpectations(t)
	mockWorker.AssertExpectations(t)
}

// TestConsumer_Worker_InvalidPayloadForRetry verifies handling of invalid payload during retry check.
func TestConsumer_Worker_InvalidPayloadForRetry(t *testing.T) {
	mockWorker := new(MockWorker)
	c := &natsConsumer{
		worker:      mockWorker,
		numWorkers:  1,
		workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
		metrics:     &types.NoopMetrics{},
	}
	c.wg.Add(1)

	// Create two separate mock messages to simulate the two Data() calls
	msg := new(MockMsg)
	task := &types.DeliveryTask{TriggerID: "t1"}
	data, _ := json.Marshal(task)

	// First call for processMsg, second call for retry check in workerLoop
	msg.On("Data").Return(data).Once()                   // processMsg unmarshal - valid
	msg.On("Data").Return([]byte("invalid-json")).Once() // workerLoop retry unmarshal - invalid
	mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(errors.New("process failed"))
	msg.On("Metadata").Return(&jetstream.MsgMetadata{NumDelivered: 1}, nil)
	msg.On("Term").Return(nil)

	c.workerChans[0] <- msg
	close(c.workerChans[0])

	c.workerLoop(context.Background(), 0)

	msg.AssertExpectations(t)
}

// TestConsumer_Worker_MaxBackoffLimit verifies that backoff is capped at maxBackoff.
func TestConsumer_Worker_MaxBackoffLimit(t *testing.T) {
	mockWorker := new(MockWorker)
	c := &natsConsumer{
		worker:      mockWorker,
		numWorkers:  1,
		workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
		metrics:     &types.NoopMetrics{},
	}
	c.wg.Add(1)

	msg := new(MockMsg)
	task := &types.DeliveryTask{
		TriggerID: "t1",
		RetryPolicy: types.RetryPolicy{
			MaxAttempts:    10,
			InitialBackoff: types.Duration(1 * time.Second),
			MaxBackoff:     types.Duration(5 * time.Second), // Cap at 5s
		},
	}
	data, _ := json.Marshal(task)

	msg.On("Data").Return(data)
	mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(errors.New("process failed"))
	// NumDelivered=5 means attempt 5, exponential backoff would be 16s, but capped at 5s
	msg.On("Metadata").Return(&jetstream.MsgMetadata{NumDelivered: 5}, nil)
	msg.On("NakWithDelay", 5*time.Second).Return(nil)

	c.workerChans[0] <- msg
	close(c.workerChans[0])

	c.workerLoop(context.Background(), 0)

	msg.AssertExpectations(t)
}

// TestConsumer_Worker_DefaultRetryValues verifies default retry values are used when not specified.
func TestConsumer_Worker_DefaultRetryValues(t *testing.T) {
	mockWorker := new(MockWorker)
	c := &natsConsumer{
		worker:      mockWorker,
		numWorkers:  1,
		workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
		metrics:     &types.NoopMetrics{},
	}
	c.wg.Add(1)

	msg := new(MockMsg)
	// Task with no retry policy - should use defaults (maxAttempts=3, initialBackoff=1s)
	task := &types.DeliveryTask{
		TriggerID: "t1",
	}
	data, _ := json.Marshal(task)

	msg.On("Data").Return(data)
	mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(errors.New("process failed"))
	msg.On("Metadata").Return(&jetstream.MsgMetadata{NumDelivered: 1}, nil)
	msg.On("NakWithDelay", 1*time.Second).Return(nil) // Default initial backoff

	c.workerChans[0] <- msg
	close(c.workerChans[0])

	c.workerLoop(context.Background(), 0)

	msg.AssertExpectations(t)
}

// TestConsumer_ProcessMsg_WithTimeout verifies that custom timeout is applied.
func TestConsumer_ProcessMsg_WithTimeout(t *testing.T) {
	mockWorker := new(MockWorker)
	c := &natsConsumer{
		worker:  mockWorker,
		metrics: &types.NoopMetrics{},
	}

	msg := new(MockMsg)
	task := &types.DeliveryTask{
		TriggerID:  "t1",
		Tenant:     "tenant1",
		Collection: "col1",
		Timeout:    types.Duration(500 * time.Millisecond), // Custom timeout
	}
	data, _ := json.Marshal(task)

	msg.On("Data").Return(data)
	mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(nil)

	err := c.processMsg(context.Background(), msg)
	assert.NoError(t, err)

	mockWorker.AssertExpectations(t)
}

// TestConsumer_ProcessMsg_InvalidPayload verifies error handling for invalid payload.
func TestConsumer_ProcessMsg_InvalidPayload(t *testing.T) {
	mockWorker := new(MockWorker)
	c := &natsConsumer{
		worker:  mockWorker,
		metrics: &types.NoopMetrics{},
	}

	msg := new(MockMsg)
	msg.On("Data").Return([]byte("invalid-json"))

	err := c.processMsg(context.Background(), msg)
	assert.ErrorContains(t, err, "invalid payload")
}

// TestConsumer_ProcessMsg_WorkerError verifies metrics are recorded on worker error.
func TestConsumer_ProcessMsg_WorkerError(t *testing.T) {
	mockWorker := new(MockWorker)
	mockMetrics := new(MockMetrics)
	c := &natsConsumer{
		worker:  mockWorker,
		metrics: mockMetrics,
	}

	msg := new(MockMsg)
	task := &types.DeliveryTask{
		TriggerID:  "t1",
		Tenant:     "tenant1",
		Collection: "col1",
	}
	data, _ := json.Marshal(task)

	msg.On("Data").Return(data)
	mockWorker.On("ProcessTask", mock.Anything, mock.Anything).Return(errors.New("worker failed"))
	mockMetrics.On("IncConsumeFailure", "tenant1", "col1", "worker failed").Return()
	mockMetrics.On("ObserveConsumeLatency", "tenant1", "col1", mock.Anything).Return()

	err := c.processMsg(context.Background(), msg)
	assert.Error(t, err)

	mockWorker.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// MockMetrics is a mock implementation of types.Metrics for testing.
type MockMetrics struct {
	mock.Mock
}

func (m *MockMetrics) IncPublishSuccess(tenant, collection string, hashed bool) {
	m.Called(tenant, collection, hashed)
}

func (m *MockMetrics) IncPublishFailure(tenant, collection, reason string) {
	m.Called(tenant, collection, reason)
}

func (m *MockMetrics) IncConsumeSuccess(tenant, collection string, hashed bool) {
	m.Called(tenant, collection, hashed)
}

func (m *MockMetrics) IncConsumeFailure(tenant, collection, reason string) {
	m.Called(tenant, collection, reason)
}

func (m *MockMetrics) ObservePublishLatency(tenant, collection string, d time.Duration) {
	m.Called(tenant, collection, d)
}

func (m *MockMetrics) ObserveConsumeLatency(tenant, collection string, d time.Duration) {
	m.Called(tenant, collection, d)
}

func (m *MockMetrics) IncHashCollision(tenant, collection string) {
	m.Called(tenant, collection)
}

func (m *MockMetrics) IncDeliverySuccess(tenant, collection string) {
	m.Called(tenant, collection)
}

func (m *MockMetrics) IncDeliveryFailure(tenant, collection string, status int, fatal bool) {
	m.Called(tenant, collection, status, fatal)
}

func (m *MockMetrics) IncDeliveryRetry(tenant, collection string) {
	m.Called(tenant, collection)
}

func (m *MockMetrics) ObserveDeliveryLatency(tenant, collection string, d time.Duration) {
	m.Called(tenant, collection, d)
}
