package engine

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/trigger/internal/pubsub"
	"github.com/syntrixbase/syntrix/internal/trigger/internal/worker"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// MockTaskConsumer
type MockTaskConsumer struct {
	mock.Mock
}

func (m *MockTaskConsumer) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestNewFactory(t *testing.T) {
	t.Parallel()
	f, err := NewFactory(nil, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, f)
}

func TestFactoryOptions(t *testing.T) {
	t.Parallel()
	f := &defaultTriggerFactory{}

	WithDatabase("database1")(f)
	assert.Equal(t, "database1", f.database)

	WithStartFromNow(true)(f)
	assert.True(t, f.startFromNow)

	m := &types.NoopMetrics{}
	WithMetrics(m)(f)
	assert.Equal(t, m, f.metrics)

	// WithSecretProvider
	// We need a mock SecretProvider or nil
	WithSecretProvider(nil)(f)
	assert.Nil(t, f.secrets)

	// WithStreamName
	WithStreamName("test-stream")(f)
	assert.Equal(t, "test-stream", f.streamName)

	// WithRulesFile
	WithRulesFile("/path/to/rules.yaml")(f)
	assert.Equal(t, "/path/to/rules.yaml", f.rulesFile)
}

func TestFactory_Engine_Success(t *testing.T) {
	t.Parallel()
	mockPuller := new(MockPullerService)
	// If nats is nil, Engine() should succeed (with nil publisher)
	f, err := NewFactory(nil, nil, nil, WithPuller(mockPuller))
	assert.NoError(t, err)
	e, err := f.Engine()
	assert.NoError(t, err)
	assert.NotNil(t, e)
}

func TestFactory_Engine_NoPuller(t *testing.T) {
	t.Parallel()
	f, err := NewFactory(nil, nil, nil)
	assert.NoError(t, err)
	e, err := f.Engine()
	assert.Error(t, err)
	assert.Nil(t, e)
	assert.Contains(t, err.Error(), "puller service is required")
}

func TestFactory_Engine_WithNATS(t *testing.T) {
	// Mock newTaskPublisher
	originalNewTaskPublisher := newTaskPublisher
	defer func() { newTaskPublisher = originalNewTaskPublisher }()

	mockPub := new(MockPublisher)
	newTaskPublisher = func(nc *nats.Conn, streamName string, metrics types.Metrics) (pubsub.TaskPublisher, error) {
		return mockPub, nil
	}

	mockPuller := new(MockPullerService)
	// Pass a dummy nats conn (can be nil if our mock doesn't check, but factory checks f.nats != nil)
	// We need f.nats != nil to trigger the branch.
	// But NewFactory takes *nats.Conn.
	// We can pass &nats.Conn{}
	f, err := NewFactory(nil, &nats.Conn{}, nil, WithPuller(mockPuller))
	assert.NoError(t, err)

	e, err := f.Engine()
	assert.NoError(t, err)
	assert.NotNil(t, e)
}

func TestFactory_Engine_PublisherFail(t *testing.T) {
	// Mock newTaskPublisher
	originalNewTaskPublisher := newTaskPublisher
	defer func() { newTaskPublisher = originalNewTaskPublisher }()

	newTaskPublisher = func(nc *nats.Conn, streamName string, metrics types.Metrics) (pubsub.TaskPublisher, error) {
		return nil, fmt.Errorf("publisher error")
	}

	mockPuller := new(MockPullerService)
	f, err := NewFactory(nil, &nats.Conn{}, nil, WithPuller(mockPuller))
	assert.NoError(t, err)

	e, err := f.Engine()
	assert.Error(t, err)
	assert.Nil(t, e)
	assert.Contains(t, err.Error(), "failed to create publisher")
}

func TestFactory_Engine_WithRulesFile(t *testing.T) {
	t.Parallel()

	// Create a temporary rules file
	tmpDir := t.TempDir()
	rulesFile := tmpDir + "/triggers.yaml"
	rulesContent := `
- triggerId: "test-trigger"
  database: "default"
  collection: "users"
  events: ["create"]
  condition: "true"
  url: "http://localhost:8080/webhook"
`
	err := os.WriteFile(rulesFile, []byte(rulesContent), 0644)
	assert.NoError(t, err)

	mockPuller := new(MockPullerService)
	f, err := NewFactory(nil, nil, nil, WithPuller(mockPuller), WithRulesFile(rulesFile))
	assert.NoError(t, err)

	e, err := f.Engine()
	assert.NoError(t, err)
	assert.NotNil(t, e)
}

func TestFactory_Engine_WithRulesFile_InvalidFile(t *testing.T) {
	t.Parallel()

	mockPuller := new(MockPullerService)
	f, err := NewFactory(nil, nil, nil, WithPuller(mockPuller), WithRulesFile("/nonexistent/path.yaml"))
	assert.NoError(t, err)

	e, err := f.Engine()
	assert.Error(t, err)
	assert.Nil(t, e)
	assert.Contains(t, err.Error(), "failed to load trigger rules")
}

func TestFactory_Consumer_Fail(t *testing.T) {
	t.Parallel()
	// If nats is nil, Consumer() should fail because NewTaskConsumer fails
	f, err := NewFactory(nil, nil, nil)
	assert.NoError(t, err)
	c, err := f.Consumer(1)
	assert.Error(t, err)
	assert.Nil(t, c)
}

func TestFactory_Consumer_Success(t *testing.T) {
	// Mock newTaskConsumer
	originalNewTaskConsumer := newTaskConsumer
	defer func() { newTaskConsumer = originalNewTaskConsumer }()

	mockConsumer := new(MockTaskConsumer)
	newTaskConsumer = func(nc *nats.Conn, w worker.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...pubsub.ConsumerOption) (pubsub.TaskConsumer, error) {
		return mockConsumer, nil
	}

	f, err := NewFactory(nil, &nats.Conn{}, nil)
	assert.NoError(t, err)

	c, err := f.Consumer(1)
	assert.NoError(t, err)
	assert.NotNil(t, c)
}

func TestFactory_Close(t *testing.T) {
	f, err := NewFactory(nil, nil, nil)
	assert.NoError(t, err)
	assert.NoError(t, f.Close())
}
