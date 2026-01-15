package evaluator

import (
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/syntrixbase/syntrix/internal/core/storage"
	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/trigger/pubsub"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
	"github.com/syntrixbase/syntrix/internal/trigger/watcher"
)

// ServiceOptions configures the evaluator service.
type ServiceOptions struct {
	StartFromNow       bool
	RulesFile          string
	StreamName         string
	CheckpointDatabase string
}

// Dependencies contains external dependencies for the evaluator service.
type Dependencies struct {
	Store   storage.DocumentStore
	Puller  puller.Service
	Nats    *nats.Conn
	Metrics types.Metrics
}

// publisherFactory is a function type for creating TaskPublisher.
// This allows injection for testing.
type publisherFactory func(nc *nats.Conn, streamName string, metrics types.Metrics) (pubsub.TaskPublisher, error)

// newTaskPublisher is the default publisher factory.
var newTaskPublisher publisherFactory = pubsub.NewTaskPublisher

// NewService creates a new evaluator Service.
func NewService(deps Dependencies, opts ServiceOptions) (Service, error) {
	if deps.Puller == nil {
		return nil, fmt.Errorf("puller service is required for evaluator service")
	}

	// Apply defaults
	if opts.StreamName == "" {
		opts.StreamName = "TRIGGERS"
	}
	if opts.CheckpointDatabase == "" {
		opts.CheckpointDatabase = "default"
	}
	if deps.Metrics == nil {
		deps.Metrics = &types.NoopMetrics{}
	}

	// Create CEL evaluator
	eval, err := NewEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to create evaluator: %w", err)
	}

	// Create watcher (receives all events; database filtering is done by Evaluator per trigger)
	w := watcher.NewWatcher(deps.Puller, deps.Store, watcher.WatcherOptions{
		StartFromNow:       opts.StartFromNow,
		CheckpointDatabase: opts.CheckpointDatabase,
	})

	// Create publisher
	var pub TaskPublisher
	if deps.Nats != nil {
		p, err := newTaskPublisher(deps.Nats, opts.StreamName, deps.Metrics)
		if err != nil {
			return nil, fmt.Errorf("failed to create publisher: %w", err)
		}
		pub = p
	}

	svc := &service{
		evaluator: eval,
		watcher:   w,
		publisher: pub,
	}

	// Load trigger rules from file if configured
	if opts.RulesFile != "" {
		triggers, err := types.LoadTriggersFromFile(opts.RulesFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load trigger rules from %s: %w", opts.RulesFile, err)
		}
		if err := svc.LoadTriggers(triggers); err != nil {
			return nil, fmt.Errorf("failed to load triggers: %w", err)
		}
	}

	return svc, nil
}
