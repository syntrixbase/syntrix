package delivery

import (
	"context"
	"fmt"

	"github.com/syntrixbase/syntrix/internal/core/identity"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
	"github.com/syntrixbase/syntrix/internal/trigger/delivery/worker"
	"github.com/syntrixbase/syntrix/internal/trigger/types"
)

// Dependencies contains external dependencies for the delivery service.
type Dependencies struct {
	Consumer pubsub.Consumer
	Auth     identity.AuthN
	Secrets  types.SecretProvider
	Metrics  types.Metrics
}

// service implements the Service interface.
type service struct {
	consumer TaskConsumer
}

// NewService creates a new delivery Service.
func NewService(deps Dependencies, opts ServiceOptions) (Service, error) {
	if deps.Consumer == nil {
		return nil, fmt.Errorf("consumer is required for delivery service")
	}

	// Apply defaults
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

	// Create consumer wrapping pubsub.Consumer
	taskConsumer := NewTaskConsumer(deps.Consumer, w, opts.NumWorkers, deps.Metrics, consumerOpts...)

	return &service{
		consumer: taskConsumer,
	}, nil
}

// Start begins consuming and processing delivery tasks.
func (s *service) Start(ctx context.Context) error {
	return s.consumer.Start(ctx)
}
