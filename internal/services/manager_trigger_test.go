package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/syntrixbase/syntrix/internal/config"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
	pubsubtesting "github.com/syntrixbase/syntrix/internal/core/pubsub/testing"
	"github.com/syntrixbase/syntrix/internal/core/storage"
	"github.com/syntrixbase/syntrix/internal/puller"
	puller_config "github.com/syntrixbase/syntrix/internal/puller/config"
	"github.com/syntrixbase/syntrix/internal/trigger"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator"
)

// mockTriggerPuller implements puller.LocalService for testing
type mockTriggerPuller struct{}

func (m *mockTriggerPuller) Subscribe(ctx context.Context, consumerID string, after string) <-chan *puller.Event {
	return nil
}
func (m *mockTriggerPuller) AddBackend(name string, client *mongo.Client, dbName string, cfg puller_config.PullerBackendConfig) error {
	return nil
}
func (m *mockTriggerPuller) Start(context.Context) error { return nil }
func (m *mockTriggerPuller) Stop(context.Context) error  { return nil }
func (m *mockTriggerPuller) BackendNames() []string      { return nil }
func (m *mockTriggerPuller) SetEventHandler(handler func(ctx context.Context, backendName string, event *puller.ChangeEvent) error) {
}
func (m *mockTriggerPuller) Replay(ctx context.Context, after map[string]string, streaming bool) (puller.Iterator, error) {
	return nil, nil
}

// setupTriggerTestFactories saves and restores all trigger-related factories
func setupTriggerTestFactories(t *testing.T) func() {
	origStorageFactory := storageFactoryFactory
	origEvalFactory := evaluatorServiceFactory
	origDeliveryFactory := deliveryServiceFactory
	origPubsubPublisher := pubsubPublisherFactory
	origPubsubConsumer := pubsubConsumerFactory
	origConnector := trigger.GetNatsConnectFunc()

	return func() {
		storageFactoryFactory = origStorageFactory
		evaluatorServiceFactory = origEvalFactory
		deliveryServiceFactory = origDeliveryFactory
		pubsubPublisherFactory = origPubsubPublisher
		pubsubConsumerFactory = origPubsubConsumer
		trigger.SetNatsConnectFunc(origConnector)
	}
}

func TestManager_InitTriggerServices_EvaluatorSuccess(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Trigger.Evaluator.RulesFile = ""
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true, RunPuller: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub publisher factory
	pubsubPublisherFactory = func(nc *nats.Conn, cfg evaluator.Config) (pubsub.Publisher, error) {
		return pubsubtesting.NewMockPublisher(), nil
	}

	// Set up fake puller service
	mgr.pullerService = &mockTriggerPuller{}

	// Mock evaluator factory
	evaluatorServiceFactory = func(deps evaluator.Dependencies, cfg evaluator.Config) (evaluator.Service, error) {
		return &fakeEvaluator{}, nil
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.NoError(t, err)
	assert.NotNil(t, mgr.triggerService)
}

func TestManager_InitTriggerServices_DeliverySuccess(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerWorker: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub consumer factory
	pubsubConsumerFactory = func(nc *nats.Conn, cfg delivery.Config) (pubsub.Consumer, error) {
		return pubsubtesting.NewMockConsumer(), nil
	}

	// Mock delivery factory
	deliveryServiceFactory = func(deps delivery.Dependencies, cfg delivery.Config) (delivery.Service, error) {
		return &fakeConsumer{}, nil
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.NoError(t, err)
	assert.NotNil(t, mgr.triggerConsumer)
}

func TestManager_InitTriggerServices_BothServices(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Trigger.Evaluator.RulesFile = ""
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true, RunTriggerWorker: true, RunPuller: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub factories
	pubsubPublisherFactory = func(nc *nats.Conn, cfg evaluator.Config) (pubsub.Publisher, error) {
		return pubsubtesting.NewMockPublisher(), nil
	}
	pubsubConsumerFactory = func(nc *nats.Conn, cfg delivery.Config) (pubsub.Consumer, error) {
		return pubsubtesting.NewMockConsumer(), nil
	}

	// Set up fake puller service
	mgr.pullerService = &mockTriggerPuller{}

	// Mock factories
	evaluatorServiceFactory = func(deps evaluator.Dependencies, cfg evaluator.Config) (evaluator.Service, error) {
		return &fakeEvaluator{}, nil
	}
	deliveryServiceFactory = func(deps delivery.Dependencies, cfg delivery.Config) (delivery.Service, error) {
		return &fakeConsumer{}, nil
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.NoError(t, err)
	assert.NotNil(t, mgr.triggerService)
	assert.NotNil(t, mgr.triggerConsumer)
}

func TestManager_InitTriggerServices_EvaluatorNoPuller(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// No puller service set - should fail
	err := mgr.initTriggerServices(context.Background(), false)
	assert.ErrorContains(t, err, "puller service is required")
}

func TestManager_InitTriggerServices_EvaluatorError(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true, RunPuller: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub publisher factory
	pubsubPublisherFactory = func(nc *nats.Conn, cfg evaluator.Config) (pubsub.Publisher, error) {
		return pubsubtesting.NewMockPublisher(), nil
	}

	// Set up fake puller service
	mgr.pullerService = &mockTriggerPuller{}

	// Mock evaluator factory to return error
	evaluatorServiceFactory = func(deps evaluator.Dependencies, cfg evaluator.Config) (evaluator.Service, error) {
		return nil, fmt.Errorf("evaluator error")
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.ErrorContains(t, err, "evaluator error")
}

func TestManager_InitTriggerServices_DeliveryError(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerWorker: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub consumer factory
	pubsubConsumerFactory = func(nc *nats.Conn, cfg delivery.Config) (pubsub.Consumer, error) {
		return pubsubtesting.NewMockConsumer(), nil
	}

	// Mock delivery factory to return error
	deliveryServiceFactory = func(deps delivery.Dependencies, cfg delivery.Config) (delivery.Service, error) {
		return nil, fmt.Errorf("delivery error")
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.ErrorContains(t, err, "delivery error")
}

func TestManager_InitTriggerServices_NatsError(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) {
		return nil, fmt.Errorf("nats error")
	})

	err := mgr.initTriggerServices(context.Background(), false)
	assert.ErrorContains(t, err, "nats error")
}

func TestManager_InitTriggerServices_StandaloneEmbeddedNATS(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Deployment.Mode = "standalone"
	cfg.Deployment.Standalone.EmbeddedNATS = true
	cfg.Deployment.Standalone.NATSDataDir = "/tmp/nats"
	mgr := NewManager(cfg, Options{Mode: ModeStandalone, RunTriggerEvaluator: true})

	// EmbeddedNATSProvider should return error since it's not implemented
	err := mgr.initTriggerServices(context.Background(), true)
	assert.Error(t, err)
	assert.ErrorIs(t, err, trigger.ErrEmbeddedNATSNotImplemented)
}

func TestManager_InitTriggerServices_WithRulesFile(t *testing.T) {
	cfg := config.LoadConfig()
	tmpDir := t.TempDir()
	jsonRules := `[{"triggerId":"t1","database":"default","collection":"users","events":["create"],"url":"http://localhost:8080/webhook"}]`
	rulesFile := filepath.Join(tmpDir, "rules.json")
	assert.NoError(t, os.WriteFile(rulesFile, []byte(jsonRules), 0644))
	cfg.Trigger.Evaluator.RulesFile = rulesFile

	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true, RunPuller: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub publisher factory
	pubsubPublisherFactory = func(nc *nats.Conn, cfg evaluator.Config) (pubsub.Publisher, error) {
		return pubsubtesting.NewMockPublisher(), nil
	}

	// Set up fake puller service
	mgr.pullerService = &mockTriggerPuller{}

	// Mock evaluator factory - verify rules file is passed
	evaluatorServiceFactory = func(deps evaluator.Dependencies, cfg evaluator.Config) (evaluator.Service, error) {
		assert.Equal(t, rulesFile, cfg.RulesFile)
		return &fakeEvaluator{}, nil
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.NoError(t, err)
}

func TestManager_InitTriggerServices_PublisherFactoryError(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.Trigger.Evaluator.RulesFile = ""
	mgr := NewManager(cfg, Options{RunTriggerEvaluator: true, RunPuller: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub publisher factory to return error
	pubsubPublisherFactory = func(nc *nats.Conn, cfg evaluator.Config) (pubsub.Publisher, error) {
		return nil, fmt.Errorf("publisher factory error")
	}

	// Set up fake puller service
	mgr.pullerService = &mockTriggerPuller{}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.ErrorContains(t, err, "publisher factory error")
}

func TestManager_InitTriggerServices_ConsumerFactoryError(t *testing.T) {
	cfg := config.LoadConfig()
	mgr := NewManager(cfg, Options{RunTriggerWorker: true})

	restore := setupTriggerTestFactories(t)
	defer restore()

	storageFactoryFactory = func(ctx context.Context, cfg *config.Config) (storage.StorageFactory, error) {
		return &fakeStorageFactory{}, nil
	}

	// Set up fake NATS connection
	fakeConn := &nats.Conn{}
	trigger.SetNatsConnectFunc(func(string, ...nats.Option) (*nats.Conn, error) { return fakeConn, nil })

	// Mock pubsub consumer factory to return error
	pubsubConsumerFactory = func(nc *nats.Conn, cfg delivery.Config) (pubsub.Consumer, error) {
		return nil, fmt.Errorf("consumer factory error")
	}

	err := mgr.initTriggerServices(context.Background(), false)
	assert.ErrorContains(t, err, "consumer factory error")
}

// Fakes

type fakeEvaluator struct{}

func (f *fakeEvaluator) LoadTriggers(triggers []*trigger.Trigger) error {
	return nil
}

func (f *fakeEvaluator) Start(ctx context.Context) error {
	return nil
}

func (f *fakeEvaluator) Close() error {
	return nil
}

type fakeConsumer struct{}

func (f *fakeConsumer) Start(ctx context.Context) error {
	return nil
}

type fakePublisher struct{ created bool }

func (f *fakePublisher) Publish(context.Context, *trigger.DeliveryTask) error {
	f.created = true
	return nil
}

func (f *fakePublisher) Close() {}
