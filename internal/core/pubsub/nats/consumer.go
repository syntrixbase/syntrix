package nats

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/syntrixbase/syntrix/internal/core/pubsub"
)

// jetStreamConsumer implements pubsub.Consumer using NATS JetStream.
type jetStreamConsumer struct {
	js          jetstream.JetStream
	opts        pubsub.ConsumerOptions
	workerChans []chan jetstream.Msg
	wg          sync.WaitGroup

	// Shutdown coordination
	closing  atomic.Bool
	inFlight atomic.Int32
}

// NewConsumer creates a new Consumer backed by NATS JetStream.
func NewConsumer(nc *nats.Conn, opts pubsub.ConsumerOptions) (pubsub.Consumer, error) {
	if nc == nil {
		return nil, fmt.Errorf("nats connection cannot be nil")
	}

	js, err := JetStreamNew(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	// Apply defaults
	if opts.NumWorkers <= 0 {
		opts.NumWorkers = pubsub.DefaultConsumerOptions().NumWorkers
	}
	if opts.ChannelBufSize <= 0 {
		opts.ChannelBufSize = pubsub.DefaultConsumerOptions().ChannelBufSize
	}
	if opts.DrainTimeout <= 0 {
		opts.DrainTimeout = pubsub.DefaultConsumerOptions().DrainTimeout
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = pubsub.DefaultConsumerOptions().ShutdownTimeout
	}
	if opts.StreamName == "" {
		return nil, fmt.Errorf("stream name is required")
	}

	return &jetStreamConsumer{js: js, opts: opts}, nil
}

// Start begins consuming messages. Blocks until context is cancelled.
func (c *jetStreamConsumer) Start(ctx context.Context, handler pubsub.MessageHandler) error {
	// Ensure stream exists
	filterSubject := c.opts.FilterSubject
	if filterSubject == "" {
		filterSubject = c.opts.StreamName + ".>"
	}

	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     c.opts.StreamName,
		Subjects: []string{filterSubject},
		Storage:  jetstream.MemoryStorage,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure stream: %w", err)
	}

	// Create durable consumer
	consumerName := c.opts.ConsumerName
	if consumerName == "" {
		consumerName = "consumer"
	}

	consumer, err := c.js.CreateOrUpdateConsumer(ctx, c.opts.StreamName, jetstream.ConsumerConfig{
		Durable:       consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: filterSubject,
	})
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}

	// Initialize worker pool
	c.workerChans = make([]chan jetstream.Msg, c.opts.NumWorkers)
	for i := 0; i < c.opts.NumWorkers; i++ {
		c.workerChans[i] = make(chan jetstream.Msg, c.opts.ChannelBufSize)
		c.wg.Add(1)
		go c.workerLoop(ctx, i, handler)
	}

	// Start consuming
	cc, err := consumer.Consume(func(msg jetstream.Msg) {
		c.dispatch(msg)
	})
	if err != nil {
		return fmt.Errorf("failed to start consumer: %w", err)
	}
	defer cc.Stop()

	log.Printf("[pubsub] Consumer started with %d workers, stream=%s", c.opts.NumWorkers, c.opts.StreamName)

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	return c.shutdown(cc)
}

// dispatch routes a message to the appropriate worker.
func (c *jetStreamConsumer) dispatch(msg jetstream.Msg) {
	c.inFlight.Add(1)
	defer c.inFlight.Add(-1)

	// Check if closing - NAK for redelivery
	if c.closing.Load() {
		log.Printf("[pubsub] Consumer closing, NAK message for redelivery")
		msg.Nak()
		return
	}

	// Determine worker index
	workerIdx := 0
	if c.opts.Partitioner != nil {
		hash := c.opts.Partitioner(msg.Data())
		workerIdx = int(hash % uint32(c.opts.NumWorkers))
	} else {
		// Round-robin based on in-flight count (simple distribution)
		workerIdx = int(c.inFlight.Load()) % c.opts.NumWorkers
	}

	// Try to send to worker channel. Use a loop with closing check to avoid
	// blocking indefinitely if shutdown happens while we're waiting.
	for {
		// Check closing state before each attempt
		if c.closing.Load() {
			log.Printf("[pubsub] Consumer closing during dispatch, NAK message for redelivery")
			msg.Nak()
			return
		}

		select {
		case c.workerChans[workerIdx] <- msg:
			// Successfully sent to worker
			return
		default:
			// Channel is full, yield and retry
			time.Sleep(time.Millisecond)
		}
	}
}

// workerLoop processes messages for a single worker.
func (c *jetStreamConsumer) workerLoop(ctx context.Context, id int, handler pubsub.MessageHandler) {
	defer c.wg.Done()

	for msg := range c.workerChans[id] {
		start := time.Now()
		wrappedMsg := WrapMessage(msg)

		err := handler(ctx, wrappedMsg)

		if c.opts.OnMessage != nil {
			c.opts.OnMessage(msg.Subject(), err, time.Since(start))
		}

		// Note: The handler is responsible for calling Ack/Nak/Term.
		// If the handler doesn't call any of these, the message will be redelivered
		// after the ack wait timeout.
	}
}

// shutdown performs graceful shutdown.
func (c *jetStreamConsumer) shutdown(cc jetstream.ConsumeContext) error {
	log.Println("[pubsub] Stopping consumer...")
	c.closing.Store(true)
	cc.Stop()

	// Wait for in-flight dispatches
	drainCtx, drainCancel := context.WithTimeout(context.Background(), c.opts.DrainTimeout)
	defer drainCancel()
	c.waitForDrain(drainCtx)

	// Close worker channels
	for _, ch := range c.workerChans {
		close(ch)
	}

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), c.opts.ShutdownTimeout)
	defer shutdownCancel()

	select {
	case <-done:
		log.Println("[pubsub] All workers stopped gracefully")
	case <-shutdownCtx.Done():
		log.Printf("[pubsub] Shutdown timeout exceeded, some workers may still be running")
	}

	return nil
}

// waitForDrain waits for all in-flight dispatch calls to complete.
func (c *jetStreamConsumer) waitForDrain(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			remaining := c.inFlight.Load()
			if remaining > 0 {
				log.Printf("[pubsub] Drain timeout, %d messages still in-flight", remaining)
			}
			return
		case <-ticker.C:
			if c.inFlight.Load() == 0 {
				log.Println("[pubsub] All in-flight messages drained")
				return
			}
		}
	}
}
