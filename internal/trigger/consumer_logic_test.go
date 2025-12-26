package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/mock"
)

// TestConsumer_Dispatch_InvalidPayload verifies that invalid payloads are Terminated.
func TestConsumer_Dispatch_InvalidPayload(t *testing.T) {
	// Setup
	c := &Consumer{
		numWorkers:  1,
		workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
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
			name:           "Retry with Backoff (Attempt 1)",
			processErr:     errors.New("process failed"),
			numDelivered:   1,
			maxAttempts:    3,
			initialBackoff: 1 * time.Second,
			payload: DeliveryTask{
				TriggerID: "t1",
				RetryPolicy: RetryPolicy{
					MaxAttempts:    3,
					InitialBackoff: Duration(1 * time.Second),
				},
			},
			expectNakDelay: 1 * time.Second, // 1 * 2^(1-1) = 1
		},
		{
			name:           "Retry with Backoff (Attempt 2)",
			processErr:     errors.New("process failed"),
			numDelivered:   2,
			maxAttempts:    3,
			initialBackoff: 1 * time.Second,
			payload: DeliveryTask{
				TriggerID: "t1",
				RetryPolicy: RetryPolicy{
					MaxAttempts:    3,
					InitialBackoff: Duration(1 * time.Second),
				},
			},
			expectNakDelay: 2 * time.Second, // 1 * 2^(2-1) = 2
		},
		{
			name:         "Max Attempts Reached",
			processErr:   errors.New("process failed"),
			numDelivered: 3,
			maxAttempts:  3,
			payload: DeliveryTask{
				TriggerID: "t1",
				RetryPolicy: RetryPolicy{
					MaxAttempts: 3,
				},
			},
			expectTerm: true,
		},
		{
			name:        "Metadata Error",
			processErr:  errors.New("process failed"),
			metadataErr: errors.New("meta error"),
			payload:     DeliveryTask{TriggerID: "t1"},
			expectNak:   true,
		},
		{
			name:       "Invalid Payload on Retry Check",
			processErr: errors.New("process failed"),
			payload:    "invalid-json",
			expectTerm: true,
		},
		{
			name:           "Max Backoff Cap",
			processErr:     errors.New("process failed"),
			numDelivered:   5,
			maxAttempts:    10,
			initialBackoff: 1 * time.Second,
			maxBackoff:     5 * time.Second,
			payload: DeliveryTask{
				TriggerID: "t1",
				RetryPolicy: RetryPolicy{
					MaxAttempts:    10,
					InitialBackoff: Duration(1 * time.Second),
					MaxBackoff:     Duration(5 * time.Second),
				},
			},
			expectNakDelay: 5 * time.Second, // Calculated would be 16s, capped at 5s
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			worker := new(MockWorker)
			c := &Consumer{
				worker:      worker,
				numWorkers:  1,
				workerChans: []chan jetstream.Msg{make(chan jetstream.Msg, 1)},
				wg:          sync.WaitGroup{},
			}
			msg := new(MockMsg)

			// Prepare Payload
			var payloadBytes []byte
			if s, ok := tc.payload.(string); ok {
				payloadBytes = []byte(s)
			} else {
				payloadBytes, _ = json.Marshal(tc.payload)
			}

			// Expectations
			// 1. ProcessMsg calls Data() and Worker.ProcessTask()
			msg.On("Data").Return(payloadBytes)

			// If payload is valid, ProcessTask is called
			if _, ok := tc.payload.(DeliveryTask); ok {
				worker.On("ProcessTask", mock.Anything, mock.Anything).Return(tc.processErr)
			} else {
				// If payload is invalid string, ProcessMsg returns error immediately without calling worker
				// But wait, workerLoop calls processMsg.
				// processMsg unmarshals. If fails, returns error.
				// Then workerLoop handles error.
			}

			// 2. Error Handling Logic
			if tc.processErr != nil || tc.payload == "invalid-json" {
				// Metadata check
				if tc.metadataErr != nil {
					msg.On("Metadata").Return(nil, tc.metadataErr)
				} else {
					msg.On("Metadata").Return(&jetstream.MsgMetadata{NumDelivered: tc.numDelivered}, nil)
				}

				// Actions
				if tc.expectNak {
					msg.On("Nak").Return(nil)
				}
				if tc.expectTerm {
					msg.On("Term").Return(nil)
				}
				if tc.expectNakDelay > 0 {
					msg.On("NakWithDelay", tc.expectNakDelay).Return(nil)
				}
			} else {
				msg.On("Ack").Return(nil)
			}

			// Run Worker Loop
			c.wg.Add(1)
			go c.workerLoop(context.Background(), 0)

			// Send Message
			c.workerChans[0] <- msg
			close(c.workerChans[0]) // Close channel to stop worker loop

			// Wait
			c.wg.Wait()

			// Verify
			msg.AssertExpectations(t)
			worker.AssertExpectations(t)
		})
	}
}
