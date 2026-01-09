// Package client implements the gRPC client for the puller service.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	pullerv1 "github.com/syntrixbase/syntrix/api/gen/puller/v1"
	"github.com/syntrixbase/syntrix/internal/puller/events"
	"github.com/syntrixbase/syntrix/internal/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ConnectionState represents the current connection state.
type ConnectionState int

const (
	// StateConnected indicates the client is connected and receiving events.
	StateConnected ConnectionState = iota
	// StateReconnecting indicates the client is attempting to reconnect.
	StateReconnecting
	// StateDisconnected indicates the client has stopped (context canceled or max retries).
	StateDisconnected
)

// StateChangeCallback is called when connection state changes.
type StateChangeCallback func(state ConnectionState, err error)

// ClientConfig configures the puller client behavior.
type ClientConfig struct {
	// InitialBackoff is the initial wait time before first reconnect attempt.
	// Defaults to 1 second.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum wait time between reconnect attempts.
	// Defaults to 30 seconds.
	MaxBackoff time.Duration
	// BackoffMultiplier is the factor by which backoff increases after each failure.
	// Defaults to 2.0.
	BackoffMultiplier float64
	// MaxRetries is the maximum number of consecutive reconnect attempts.
	// Set to 0 for unlimited retries. Defaults to 0 (unlimited).
	MaxRetries int
	// OnStateChange is called when connection state changes. Optional.
	OnStateChange StateChangeCallback
}

// DefaultClientConfig returns sensible defaults for client configuration.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		MaxRetries:        0, // unlimited
	}
}

// Client is a gRPC client for the puller service with automatic reconnection.
type Client struct {
	address string
	conn    *grpc.ClientConn
	client  pullerv1.PullerServiceClient
	logger  *slog.Logger
	cfg     ClientConfig
	mu      sync.RWMutex
}

// New creates a new puller client with default configuration.
func New(address string, logger *slog.Logger) (*Client, error) {
	return NewWithConfig(address, logger, DefaultClientConfig())
}

// NewWithConfig creates a new puller client with custom configuration.
func NewWithConfig(address string, logger *slog.Logger, cfg ClientConfig) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Apply defaults
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 1 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.BackoffMultiplier <= 0 {
		cfg.BackoffMultiplier = 2.0
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to puller service: %w", err)
	}

	return &Client{
		address: address,
		conn:    conn,
		client:  pullerv1.NewPullerServiceClient(conn),
		logger:  logger.With("component", "puller-client"),
		cfg:     cfg,
	}, nil
}

// Subscribe subscribes to events from the puller service with automatic reconnection.
// The after parameter is the progress marker to resume from.
// Returns a channel of events that will be closed when the context is canceled
// or max retries is reached.
//
// The subscription automatically:
// - Reconnects on connection failures with exponential backoff
// - Resumes from the last received progress marker
// - Filters out heartbeat events (nil ChangeEvent)
func (c *Client) Subscribe(ctx context.Context, consumerID string, after string) <-chan *events.PullerEvent {
	return c.subscribe(ctx, consumerID, after, false)
}

// SubscribeWithCoalesce subscribes with catch-up coalescing enabled.
// When catching up, multiple events for the same document may be merged.
func (c *Client) SubscribeWithCoalesce(ctx context.Context, consumerID string, after string) <-chan *events.PullerEvent {
	return c.subscribe(ctx, consumerID, after, true)
}

// subscribe is the internal implementation with reconnection logic.
func (c *Client) subscribe(ctx context.Context, consumerID string, after string, coalesce bool) <-chan *events.PullerEvent {
	ch := make(chan *events.PullerEvent, 1000)

	go c.subscribeLoop(ctx, consumerID, after, coalesce, ch)

	return ch
}

// subscribeLoop handles the reconnection logic.
func (c *Client) subscribeLoop(
	ctx context.Context,
	consumerID string,
	initialAfter string,
	coalesce bool,
	ch chan *events.PullerEvent,
) {
	defer close(ch)

	currentProgress := initialAfter
	consecutiveFailures := 0
	backoff := c.cfg.InitialBackoff

	notifyState := func(state ConnectionState, err error) {
		if c.cfg.OnStateChange != nil {
			c.cfg.OnStateChange(state, err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			notifyState(StateDisconnected, ctx.Err())
			return
		default:
		}

		// Create subscription request
		req := &pullerv1.SubscribeRequest{
			ConsumerId:        consumerID,
			After:             currentProgress,
			CoalesceOnCatchUp: coalesce,
		}

		c.mu.RLock()
		client := c.client
		c.mu.RUnlock()

		stream, err := client.Subscribe(ctx, req)
		if err != nil {
			c.logger.Error("failed to subscribe", "error", err, "attempt", consecutiveFailures+1)
			notifyState(StateReconnecting, err)

			consecutiveFailures++
			if c.cfg.MaxRetries > 0 && consecutiveFailures >= c.cfg.MaxRetries {
				c.logger.Error("max reconnect attempts reached", "maxRetries", c.cfg.MaxRetries)
				notifyState(StateDisconnected, fmt.Errorf("max reconnect attempts reached: %w", err))
				return
			}

			// Wait with backoff
			select {
			case <-ctx.Done():
				notifyState(StateDisconnected, ctx.Err())
				return
			case <-time.After(backoff):
			}

			// Increase backoff
			backoff = time.Duration(float64(backoff) * c.cfg.BackoffMultiplier)
			if backoff > c.cfg.MaxBackoff {
				backoff = c.cfg.MaxBackoff
			}

			// Try to reconnect
			if reconnErr := c.reconnect(); reconnErr != nil {
				c.logger.Error("reconnection failed", "error", reconnErr)
			}

			continue
		}

		// Successfully connected
		notifyState(StateConnected, nil)
		consecutiveFailures = 0
		backoff = c.cfg.InitialBackoff

		c.logger.Info("subscription stream established",
			"consumerID", consumerID,
			"after", currentProgress,
		)

		// Read events from stream
		for {
			evt, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					c.logger.Info("subscription stream closed by server")
				} else if ctx.Err() != nil {
					// Context was canceled
					notifyState(StateDisconnected, ctx.Err())
					return
				} else {
					c.logger.Error("subscription stream error", "error", err)
				}

				notifyState(StateReconnecting, err)
				break // Break inner loop to reconnect
			}

			// Update progress marker (always, even for heartbeats)
			if evt.Progress != "" {
				currentProgress = evt.Progress
			}

			// Check if this is a heartbeat (nil ChangeEvent)
			if evt.ChangeEvent == nil {
				c.logger.Debug("received heartbeat", "progress", evt.Progress)
				continue // Skip heartbeats, don't send to channel
			}

			// Convert and send event
			normalized := c.convertEvent(evt)
			select {
			case ch <- normalized:
			case <-ctx.Done():
				notifyState(StateDisconnected, ctx.Err())
				return
			}
		}

		// Stream ended, wait before reconnecting
		select {
		case <-ctx.Done():
			notifyState(StateDisconnected, ctx.Err())
			return
		case <-time.After(backoff):
		}
	}
}

// reconnect attempts to re-establish the gRPC connection.
func (c *Client) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close existing connection if any
	if c.conn != nil {
		_ = c.conn.Close()
	}

	conn, err := grpc.NewClient(c.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	c.conn = conn
	c.client = pullerv1.NewPullerServiceClient(conn)
	return nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// convertEvent converts a gRPC event to a PullerEvent.
func (c *Client) convertEvent(evt *pullerv1.PullerEvent) *events.PullerEvent {
	change := evt.ChangeEvent

	normalized := &events.StoreChangeEvent{
		EventID:    change.EventId,
		DatabaseID: change.Database,
		MgoColl:    change.MgoColl,
		MgoDocID:   change.MgoDocId,
		OpType:     events.StoreOperationType(change.OpType),
		Timestamp:  change.Timestamp,
		TxnNumber:  &change.TxnNumber,
		Backend:    change.Backend,
	}

	if change.ClusterTime != nil {
		normalized.ClusterTime = events.ClusterTime{
			T: change.ClusterTime.T,
			I: change.ClusterTime.I,
		}
	}

	if len(change.FullDoc) > 0 {
		var doc storage.StoredDoc
		if err := json.Unmarshal(change.FullDoc, &doc); err == nil {
			normalized.FullDocument = &doc
		} else {
			c.logger.Error("failed to unmarshal full document", "error", err)
		}
	}

	if len(change.UpdateDesc) > 0 {
		var desc events.UpdateDescription
		if err := json.Unmarshal(change.UpdateDesc, &desc); err == nil {
			normalized.UpdateDesc = &desc
		} else {
			c.logger.Error("failed to unmarshal update description", "error", err)
		}
	}

	return &events.PullerEvent{
		Change:   normalized,
		Progress: evt.Progress,
	}
}
