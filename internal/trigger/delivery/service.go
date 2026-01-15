package delivery

import (
	"context"
	"time"
)

// Service consumes delivery tasks and executes HTTP webhooks.
type Service interface {
	// Start begins consuming and processing delivery tasks.
	// Blocks until context is cancelled.
	Start(ctx context.Context) error
}

// ServiceOptions configures the delivery service.
type ServiceOptions struct {
	StreamName      string
	NumWorkers      int
	ChannelBufSize  int
	DrainTimeout    time.Duration
	ShutdownTimeout time.Duration
}
