package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
)

// jetStreamPublisher implements pubsub.Publisher using NATS JetStream.
type jetStreamPublisher struct {
	js   jetstream.JetStream
	opts pubsub.PublisherOptions
}

// NewPublisher creates a new Publisher backed by NATS JetStream.
func NewPublisher(nc *nats.Conn, opts pubsub.PublisherOptions) (pubsub.Publisher, error) {
	if nc == nil {
		return nil, fmt.Errorf("nats connection cannot be nil")
	}

	js, err := JetStreamNew(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	// Ensure stream exists
	if opts.StreamName != "" {
		subjects := []string{opts.StreamName + ".>"}
		if opts.SubjectPrefix != "" && opts.SubjectPrefix != opts.StreamName {
			subjects = []string{opts.SubjectPrefix + ".>"}
		}

		_, err = js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
			Name:     opts.StreamName,
			Subjects: subjects,
			Storage:  jetstream.MemoryStorage,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to ensure stream: %w", err)
		}
	}

	return &jetStreamPublisher{js: js, opts: opts}, nil
}

// Publish sends a message to the specified subject.
func (p *jetStreamPublisher) Publish(ctx context.Context, subject string, data []byte) error {
	start := time.Now()

	fullSubject := subject
	if p.opts.SubjectPrefix != "" {
		fullSubject = p.opts.SubjectPrefix + "." + subject
	}

	_, err := p.js.Publish(ctx, fullSubject, data)

	if p.opts.OnPublish != nil {
		p.opts.OnPublish(fullSubject, err, time.Since(start))
	}

	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", fullSubject, err)
	}

	return nil
}

// Close releases resources.
func (p *jetStreamPublisher) Close() error {
	// JetStream doesn't need explicit close
	return nil
}
