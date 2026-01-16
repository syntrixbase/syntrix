package engine

import (
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/core/storage"
	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/trigger"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery/worker"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator"
	"github.com/syntrixbase/syntrix/internal/trigger/evaluator/watcher"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

var (
	newTaskPublisher = evaluator.NewTaskPublisher
	newTaskConsumer  = func(nc *nats.Conn, w worker.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...delivery.ConsumerOption) (delivery.TaskConsumer, error) {
		return delivery.NewTaskConsumer(nc, w, streamName, numWorkers, metrics, opts...)
	}
	loadTriggersFromFile = trigger.LoadTriggersFromFile
)

// FactoryOption configures the factory.
type FactoryOption func(*defaultTriggerFactory)

// WithCheckpointDatabase sets the database for storing checkpoints.
func WithCheckpointDatabase(database string) FactoryOption {
	return func(f *defaultTriggerFactory) {
		f.checkpointDatabase = database
	}
}

// WithPuller sets the puller service for the factory.
func WithPuller(p puller.Service) FactoryOption {
	return func(f *defaultTriggerFactory) {
		f.puller = p
	}
}

// WithStartFromNow sets whether to start watching from now if checkpoint is missing.
func WithStartFromNow(start bool) FactoryOption {
	return func(f *defaultTriggerFactory) {
		f.startFromNow = start
	}
}

// WithMetrics sets the metrics provider for the factory.
func WithMetrics(m types.Metrics) FactoryOption {
	return func(f *defaultTriggerFactory) {
		f.metrics = m
	}
}

// WithSecretProvider sets the secret provider for the factory.
func WithSecretProvider(s worker.SecretProvider) FactoryOption {
	return func(f *defaultTriggerFactory) {
		f.secrets = s
	}
}

// WithStreamName sets the NATS stream name for the factory.
func WithStreamName(name string) FactoryOption {
	return func(f *defaultTriggerFactory) {
		if name != "" {
			f.streamName = name
		}
	}
}

// WithRulesFile sets the trigger rules file path for the factory.
func WithRulesFile(path string) FactoryOption {
	return func(f *defaultTriggerFactory) {
		f.rulesFile = path
	}
}

// defaultTriggerFactory implements TriggerFactory.
type defaultTriggerFactory struct {
	store              storage.DocumentStore
	nats               *nats.Conn
	auth               identity.AuthN
	puller             puller.Service
	checkpointDatabase string
	startFromNow       bool
	metrics            types.Metrics
	secrets            worker.SecretProvider
	streamName         string
	rulesFile          string
}

// NewFactory creates a new TriggerFactory.
func NewFactory(store storage.DocumentStore, nats *nats.Conn, auth identity.AuthN, opts ...FactoryOption) (TriggerFactory, error) {
	f := &defaultTriggerFactory{
		store:              store,
		nats:               nats,
		auth:               auth,
		checkpointDatabase: "default",
		metrics:            &types.NoopMetrics{},
		streamName:         "TRIGGERS",
	}
	for _, opt := range opts {
		opt(f)
	}
	return f, nil
}

// Engine returns a new TriggerEngine.
func (f *defaultTriggerFactory) Engine() (TriggerEngine, error) {
	if f.puller == nil {
		return nil, fmt.Errorf("puller service is required for trigger engine")
	}

	eval, err := evaluator.NewEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to create evaluator: %w", err)
	}

	w := watcher.NewWatcher(f.puller, f.store, watcher.WatcherOptions{
		StartFromNow:       f.startFromNow,
		CheckpointDatabase: f.checkpointDatabase,
	})

	var pub evaluator.TaskPublisher
	if f.nats != nil {
		p, err := newTaskPublisher(f.nats, f.streamName, f.metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to create publisher: %w", err)
		}
		pub = p
	}

	engine := &defaultTriggerEngine{
		evaluator: eval,
		watcher:   w,
		publisher: pub,
	}

	// Load trigger rules from file if configured
	if f.rulesFile != "" {
		triggers, err := loadTriggersFromFile(f.rulesFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load trigger rules from %s: %w", f.rulesFile, err)
		}
		if err := engine.LoadTriggers(triggers); err != nil {
			return nil, fmt.Errorf("failed to load triggers: %w", err)
		}
	}

	return engine, nil
}

// Consumer returns a new TaskConsumer.
func (f *defaultTriggerFactory) Consumer(numWorkers int) (TaskConsumer, error) {
	if f.nats == nil {
		return nil, fmt.Errorf("nats connection is required for consumer")
	}

	w := worker.NewDeliveryWorker(f.auth, f.secrets, worker.HTTPClientOptions{}, f.metrics)

	return newTaskConsumer(f.nats, w, f.streamName, numWorkers, f.metrics)
}

// Close releases resources held by the factory.
// Note: The factory does NOT own the NATS connection - it is the caller's
// responsibility to manage the NATS connection lifecycle. This design allows
// the NATS connection to be shared across multiple components.
func (f *defaultTriggerFactory) Close() error {
	// Factory does not own any resources that need explicit cleanup.
	// The NATS connection is managed by the caller (ServiceManager).
	return nil
}
