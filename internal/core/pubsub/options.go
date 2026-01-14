package pubsub

import "time"

// PublisherOptions configures publisher behavior.
type PublisherOptions struct {
	// StreamName is the name of the stream to publish to.
	StreamName string

	// SubjectPrefix is prepended to all subjects.
	SubjectPrefix string

	// OnPublish is called after each publish attempt (for metrics).
	OnPublish func(subject string, err error, latency time.Duration)
}

// ConsumerOptions configures consumer behavior.
type ConsumerOptions struct {
	// StreamName is the name of the stream to consume from.
	StreamName string

	// ConsumerName is the durable consumer name.
	ConsumerName string

	// FilterSubject filters messages by subject pattern.
	FilterSubject string

	// NumWorkers is the number of worker goroutines.
	NumWorkers int

	// ChannelBufSize is the buffer size for worker channels.
	ChannelBufSize int

	// DrainTimeout is the maximum time to wait for in-flight messages to complete.
	DrainTimeout time.Duration

	// ShutdownTimeout is the maximum time to wait for workers to finish.
	ShutdownTimeout time.Duration

	// Partitioner assigns messages to workers. If nil, round-robin is used.
	Partitioner Partitioner

	// OnMessage is called after each message is processed (for metrics).
	OnMessage func(subject string, err error, latency time.Duration)
}

// Partitioner determines which worker handles a message.
type Partitioner func(data []byte) uint32

// DefaultConsumerOptions returns ConsumerOptions with sensible defaults.
func DefaultConsumerOptions() ConsumerOptions {
	return ConsumerOptions{
		NumWorkers:      16,
		ChannelBufSize:  100,
		DrainTimeout:    5 * time.Second,
		ShutdownTimeout: 30 * time.Second,
	}
}
