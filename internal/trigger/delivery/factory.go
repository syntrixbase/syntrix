package delivery

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery/worker"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// Dependencies contains external dependencies for the delivery service.
type Dependencies struct {
	Nats    *nats.Conn
	Auth    identity.AuthN
	Secrets types.SecretProvider
	Metrics types.Metrics
}

// service implements the Service interface.
type service struct {
	consumer TaskConsumer
}

// consumerFactory is a function type for creating TaskConsumer.
// This allows injection for testing.
type consumerFactory func(nc *nats.Conn, worker types.DeliveryWorker, streamName string, numWorkers int, metrics types.Metrics, opts ...ConsumerOption) (TaskConsumer, error)

// newTaskConsumer is the default consumer factory.
var newTaskConsumer consumerFactory = NewTaskConsumer

// NewService creates a new delivery Service.
func NewService(deps Dependencies, opts ServiceOptions) (Service, error) {
	if deps.Nats == nil {
		return nil, fmt.Errorf("nats connection is required for delivery service")
	}

	// Apply defaults
	if opts.StreamName == "" {
		opts.StreamName = "TRIGGERS"
	}
	if opts.NumWorkers <= 0 {
		opts.NumWorkers = 16
	}
	if deps.Metrics == nil {
		deps.Metrics = &types.NoopMetrics{}
	}

	// Create HTTP delivery worker
	w := worker.NewDeliveryWorker(deps.Auth, deps.Secrets, worker.HTTPClientOptions{}, deps.Metrics)

	// Build consumer options
	var consumerOpts []ConsumerOption
	if opts.ChannelBufSize > 0 {
		consumerOpts = append(consumerOpts, WithChannelBufferSize(opts.ChannelBufSize))
	}
	if opts.DrainTimeout > 0 {
		consumerOpts = append(consumerOpts, WithDrainTimeout(opts.DrainTimeout))
	}
	if opts.ShutdownTimeout > 0 {
		consumerOpts = append(consumerOpts, WithShutdownTimeout(opts.ShutdownTimeout))
	}

	// Create consumer
	consumer, err := newTaskConsumer(deps.Nats, w, opts.StreamName, opts.NumWorkers, deps.Metrics, consumerOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &service{
		consumer: consumer,
	}, nil
}

// Start begins consuming and processing delivery tasks.
func (s *service) Start(ctx context.Context) error {
	return s.consumer.Start(ctx)
}
