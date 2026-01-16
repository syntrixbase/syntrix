package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// MockTaskConsumer mocks TaskConsumer
type MockTaskConsumer struct {
	mock.Mock
}

func (m *MockTaskConsumer) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestNewService_NilNats(t *testing.T) {
	deps := Dependencies{
		Nats: nil,
	}
	opts := ServiceOptions{}

	_, err := NewService(deps, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nats connection is required")
}

func TestNewService_Success(t *testing.T) {
	// Save original and restore after test
	originalFactory := newTaskConsumer
	defer func() { newTaskConsumer = originalFactory }()

	mockConsumer := new(MockTaskConsumer)
	newTaskConsumer = func(nc *nats.Conn, worker types.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...ConsumerOption) (TaskConsumer, error) {
		return mockConsumer, nil
	}

	// Create a fake NATS connection (just need non-nil for the check)
	deps := Dependencies{
		Nats:    &nats.Conn{},
		Metrics: &types.NoopMetrics{},
	}
	opts := ServiceOptions{
		StreamName: "TEST_STREAM",
		NumWorkers: 4,
	}

	svc, err := NewService(deps, opts)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNewService_WithDefaults(t *testing.T) {
	// Save original and restore after test
	originalFactory := newTaskConsumer
	defer func() { newTaskConsumer = originalFactory }()

	mockConsumer := new(MockTaskConsumer)
	newTaskConsumer = func(nc *nats.Conn, worker types.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...ConsumerOption) (TaskConsumer, error) {
		// Verify defaults were applied
		assert.Equal(t, "TRIGGERS", streamName)
		assert.Equal(t, 16, numWorkers)
		return mockConsumer, nil
	}

	deps := Dependencies{
		Nats: &nats.Conn{},
	}
	opts := ServiceOptions{} // No options - should use defaults

	svc, err := NewService(deps, opts)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNewService_WithAllOptions(t *testing.T) {
	// Save original and restore after test
	originalFactory := newTaskConsumer
	defer func() { newTaskConsumer = originalFactory }()

	mockConsumer := new(MockTaskConsumer)
	optsCaptured := false
	newTaskConsumer = func(nc *nats.Conn, worker types.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...ConsumerOption) (TaskConsumer, error) {
		// 3 options should be passed (ChannelBufSize, DrainTimeout, ShutdownTimeout)
		assert.Len(t, opts, 3)
		optsCaptured = true
		return mockConsumer, nil
	}

	deps := Dependencies{
		Nats:    &nats.Conn{},
		Metrics: &types.NoopMetrics{},
	}
	opts := ServiceOptions{
		StreamName:      "MY_STREAM",
		NumWorkers:      8,
		ChannelBufSize:  100,
		DrainTimeout:    5 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	svc, err := NewService(deps, opts)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
	assert.True(t, optsCaptured)
}

func TestNewService_ConsumerError(t *testing.T) {
	// Save original and restore after test
	originalFactory := newTaskConsumer
	defer func() { newTaskConsumer = originalFactory }()

	newTaskConsumer = func(nc *nats.Conn, worker types.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...ConsumerOption) (TaskConsumer, error) {
		return nil, errors.New("consumer creation failed")
	}

	deps := Dependencies{
		Nats: &nats.Conn{},
	}
	opts := ServiceOptions{}

	_, err := NewService(deps, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create consumer")
}

func TestService_Start(t *testing.T) {
	mockConsumer := new(MockTaskConsumer)
	svc := &service{
		consumer: mockConsumer,
	}

	mockConsumer.On("Start", mock.Anything).Return(nil)

	err := svc.Start(context.Background())
	assert.NoError(t, err)
	mockConsumer.AssertExpectations(t)
}

func TestService_Start_Error(t *testing.T) {
	mockConsumer := new(MockTaskConsumer)
	svc := &service{
		consumer: mockConsumer,
	}

	mockConsumer.On("Start", mock.Anything).Return(errors.New("consumer error"))

	err := svc.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "consumer error")
	mockConsumer.AssertExpectations(t)
}
