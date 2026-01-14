// Package pubsub provides a generic pub/sub abstraction for message-based communication.
package pubsub

import (
	"context"
	"time"
)

// Message represents a received message with acknowledgment controls.
type Message interface {
	// Data returns the raw message payload.
	Data() []byte

	// Subject returns the message subject/topic.
	Subject() string

	// Ack acknowledges successful processing.
	Ack() error

	// Nak signals processing failure, requesting redelivery.
	Nak() error

	// NakWithDelay requests redelivery after a delay.
	NakWithDelay(delay time.Duration) error

	// Term terminates the message (no redelivery).
	Term() error

	// Metadata returns delivery metadata.
	Metadata() (MessageMetadata, error)
}

// MessageMetadata contains delivery information about a message.
type MessageMetadata struct {
	NumDelivered uint64
	Timestamp    time.Time
	Subject      string
	Stream       string
	Consumer     string
}

// Publisher publishes messages to a stream.
type Publisher interface {
	// Publish sends a message to the specified subject.
	Publish(ctx context.Context, subject string, data []byte) error

	// Close releases resources.
	Close() error
}

// MessageHandler processes a single message.
// Return nil to acknowledge, error to trigger retry logic.
type MessageHandler func(ctx context.Context, msg Message) error

// Consumer consumes messages from a stream.
type Consumer interface {
	// Start begins consuming messages. Blocks until context is cancelled.
	Start(ctx context.Context, handler MessageHandler) error
}
