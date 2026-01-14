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

	// ChannelBufSize is the buffer size for the message channel.
	ChannelBufSize int
}

// DefaultConsumerOptions returns ConsumerOptions with sensible defaults.
func DefaultConsumerOptions() ConsumerOptions {
	return ConsumerOptions{
		ChannelBufSize: 100,
	}
}
