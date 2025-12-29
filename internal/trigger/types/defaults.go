package types

import "time"

// Default timeout values for trigger processing.
const (
	// DefaultTaskTimeout is the default timeout for processing a single task.
	DefaultTaskTimeout = 10 * time.Second

	// DefaultHTTPTimeout is the default timeout for HTTP requests to webhooks.
	DefaultHTTPTimeout = 5 * time.Second

	// DefaultDrainTimeout is the default timeout for draining in-flight messages during shutdown.
	DefaultDrainTimeout = 5 * time.Second

	// DefaultShutdownTimeout is the default timeout for waiting workers to finish during shutdown.
	DefaultShutdownTimeout = 10 * time.Second
)
