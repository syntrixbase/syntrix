package engine

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/puller/events"
	"github.com/syntrixbase/syntrix/internal/trigger"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// MockPullerService
type MockPullerService struct {
	mock.Mock
}

func (m *MockPullerService) Subscribe(ctx context.Context, consumerID string, after string) <-chan *puller.Event {
	args := m.Called(ctx, consumerID, after)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(<-chan *puller.Event)
}

// MockEvaluator
type MockEvaluator struct {
	mock.Mock
}

func (m *MockEvaluator) Evaluate(ctx context.Context, t *trigger.Trigger, event events.SyntrixChangeEvent) (bool, error) {
	args := m.Called(ctx, t, event)
	return args.Bool(0), args.Error(1)
}

// MockWatcher
type MockWatcher struct {
	mock.Mock
}

func (m *MockWatcher) Watch(ctx context.Context) (<-chan events.SyntrixChangeEvent, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan events.SyntrixChangeEvent), args.Error(1)
}

func (m *MockWatcher) SaveCheckpoint(ctx context.Context, token interface{}) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockWatcher) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockPublisher
type MockPublisher struct {
	mock.Mock
}

func (m *MockPublisher) Publish(ctx context.Context, task *types.DeliveryTask) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockPublisher) Close() error {
	args := m.Called()
	return args.Error(0)
}
