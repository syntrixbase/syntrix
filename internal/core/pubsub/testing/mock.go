// Package testing provides mock implementations of pubsub interfaces for testing.
package testing

import (
	"context"
	"sync"
	"time"

	"github.com/syntrixbase/syntrix/internal/core/pubsub"
)

// PublishedMessage represents a message that was published.
type PublishedMessage struct {
	Subject string
	Data    []byte
}

// MockPublisher is a mock implementation of pubsub.Publisher.
type MockPublisher struct {
	mu       sync.Mutex
	messages []PublishedMessage
	err      error
	closed   bool
}

// NewMockPublisher creates a new MockPublisher.
func NewMockPublisher() *MockPublisher {
	return &MockPublisher{}
}

// Publish records the message.
func (m *MockPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.messages = append(m.messages, PublishedMessage{
		Subject: subject,
		Data:    append([]byte(nil), data...), // Copy to avoid mutation
	})
	return nil
}

// Close marks the publisher as closed.
func (m *MockPublisher) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// Messages returns all published messages.
func (m *MockPublisher) Messages() []PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PublishedMessage(nil), m.messages...)
}

// SetError sets an error to return on Publish.
func (m *MockPublisher) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// Reset clears all messages and errors.
func (m *MockPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
	m.err = nil
	m.closed = false
}

// IsClosed returns whether Close was called.
func (m *MockPublisher) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// MockMessage is a mock implementation of pubsub.Message.
type MockMessage struct {
	mu          sync.Mutex
	data        []byte
	subject     string
	metadata    pubsub.MessageMetadata
	acked       bool
	naked       bool
	nakDelay    time.Duration
	termed      bool
	ackErr      error
	nakErr      error
	termErr     error
	metadataErr error
}

// NewMockMessage creates a new MockMessage.
func NewMockMessage(subject string, data []byte) *MockMessage {
	return &MockMessage{
		subject: subject,
		data:    data,
		metadata: pubsub.MessageMetadata{
			NumDelivered: 1,
			Timestamp:    time.Now(),
			Subject:      subject,
		},
	}
}

// Data returns the message data.
func (m *MockMessage) Data() []byte {
	return m.data
}

// Subject returns the message subject.
func (m *MockMessage) Subject() string {
	return m.subject
}

// Ack acknowledges the message.
func (m *MockMessage) Ack() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ackErr != nil {
		return m.ackErr
	}
	m.acked = true
	return nil
}

// Nak signals processing failure.
func (m *MockMessage) Nak() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nakErr != nil {
		return m.nakErr
	}
	m.naked = true
	return nil
}

// NakWithDelay signals processing failure with delay.
func (m *MockMessage) NakWithDelay(delay time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nakErr != nil {
		return m.nakErr
	}
	m.naked = true
	m.nakDelay = delay
	return nil
}

// Term terminates the message.
func (m *MockMessage) Term() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.termErr != nil {
		return m.termErr
	}
	m.termed = true
	return nil
}

// Metadata returns message metadata.
func (m *MockMessage) Metadata() (pubsub.MessageMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.metadataErr != nil {
		return pubsub.MessageMetadata{}, m.metadataErr
	}
	return m.metadata, nil
}

// IsAcked returns whether Ack was called.
func (m *MockMessage) IsAcked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acked
}

// IsNaked returns whether Nak was called.
func (m *MockMessage) IsNaked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.naked
}

// IsTermed returns whether Term was called.
func (m *MockMessage) IsTermed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.termed
}

// NakDelay returns the delay passed to NakWithDelay.
func (m *MockMessage) NakDelay() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nakDelay
}

// SetMetadata sets the metadata to return.
func (m *MockMessage) SetMetadata(md pubsub.MessageMetadata) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metadata = md
}

// SetErrors sets errors to return from various methods.
func (m *MockMessage) SetErrors(ack, nak, term, metadata error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ackErr = ack
	m.nakErr = nak
	m.termErr = term
	m.metadataErr = metadata
}

// MockConsumer is a mock implementation of pubsub.Consumer.
type MockConsumer struct {
	mu      sync.Mutex
	handler pubsub.MessageHandler
	started bool
	err     error
}

// NewMockConsumer creates a new MockConsumer.
func NewMockConsumer() *MockConsumer {
	return &MockConsumer{}
}

// Start records the handler and blocks until context is cancelled.
func (c *MockConsumer) Start(ctx context.Context, handler pubsub.MessageHandler) error {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.handler = handler
	c.started = true
	c.mu.Unlock()

	<-ctx.Done()
	return nil
}

// SimulateMessage simulates receiving a message.
func (c *MockConsumer) SimulateMessage(ctx context.Context, msg pubsub.Message) error {
	c.mu.Lock()
	h := c.handler
	c.mu.Unlock()

	if h == nil {
		return nil
	}
	return h(ctx, msg)
}

// IsStarted returns whether Start was called.
func (c *MockConsumer) IsStarted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started
}

// SetError sets an error to return from Start.
func (c *MockConsumer) SetError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}
