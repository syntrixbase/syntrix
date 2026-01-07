package streamer

import (
	"context"
	"io"
	"sync"

	"github.com/syntrixbase/syntrix/pkg/model"
)

// localStream implements the Stream interface for local (in-process) communication.
// It provides direct synchronous calls to the subscription handler, avoiding
// unnecessary message serialization and channel passing.
type localStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	gatewayID string
	handler   subscriptionHandler

	// outgoing delivers events to the consumer
	outgoing chan *EventDelivery

	closed   bool
	closedMu sync.Mutex
}

func newLocalStream(ctx context.Context, gatewayID string, handler subscriptionHandler) *localStream {
	ctx, cancel := context.WithCancel(ctx)
	return &localStream{
		ctx:       ctx,
		cancel:    cancel,
		gatewayID: gatewayID,
		handler:   handler,
		outgoing:  make(chan *EventDelivery, 1000),
	}
}

// Subscribe creates a new subscription and returns the subscription ID.
func (ls *localStream) Subscribe(tenant, collection string, filters []model.Filter) (string, error) {
	ls.closedMu.Lock()
	if ls.closed {
		ls.closedMu.Unlock()
		return "", io.EOF
	}
	ls.closedMu.Unlock()

	return ls.handler.subscribe(ls.gatewayID, tenant, collection, filters)
}

// Unsubscribe removes a subscription by ID.
func (ls *localStream) Unsubscribe(subscriptionID string) error {
	ls.closedMu.Lock()
	if ls.closed {
		ls.closedMu.Unlock()
		return io.EOF
	}
	ls.closedMu.Unlock()

	return ls.handler.unsubscribe(subscriptionID)
}

// Recv receives an EventDelivery from the Streamer.
func (ls *localStream) Recv() (*EventDelivery, error) {
	select {
	case delivery, ok := <-ls.outgoing:
		if !ok {
			return nil, io.EOF
		}
		return delivery, nil
	case <-ls.ctx.Done():
		return nil, ls.ctx.Err()
	}
}

// Close closes the stream and releases resources.
func (ls *localStream) Close() error {
	ls.closedMu.Lock()
	defer ls.closedMu.Unlock()

	if !ls.closed {
		ls.closed = true
		ls.cancel()
	}
	return nil
}

func (ls *localStream) close() {
	ls.closedMu.Lock()
	if !ls.closed {
		ls.closed = true
		ls.cancel()
	}
	ls.closedMu.Unlock()
}

// Compile-time check
var _ Stream = (*localStream)(nil)
