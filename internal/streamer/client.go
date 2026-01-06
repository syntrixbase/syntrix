package streamer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "github.com/syntrixbase/syntrix/api/gen/streamer/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientConfig configures the Streamer client.
type ClientConfig struct {
	// StreamerAddr is the address of the Streamer gRPC server.
	StreamerAddr string

	// ReconnectInterval is the time between reconnection attempts.
	ReconnectInterval time.Duration

	// HeartbeatInterval is the time between heartbeat messages.
	HeartbeatInterval time.Duration
}

// DefaultClientConfig returns sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		StreamerAddr:      "localhost:50052",
		ReconnectInterval: 5 * time.Second,
		HeartbeatInterval: 30 * time.Second,
	}
}

// streamerClient implements the Service interface for remote Streamer.
type streamerClient struct {
	config ClientConfig
	logger *slog.Logger

	conn   *grpc.ClientConn
	client pb.StreamerServiceClient

	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new streamerClient and returns the Service interface.
func NewClient(config ClientConfig, logger *slog.Logger) (Service, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if config.ReconnectInterval == 0 {
		config.ReconnectInterval = 5 * time.Second
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &streamerClient{
		config: config,
		logger: logger.With("component", "streamer-client"),
		ctx:    ctx,
		cancel: cancel,
	}

	// Connect to the remote Streamer
	conn, err := grpc.NewClient(config.StreamerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to streamer: %w", err)
	}

	c.conn = conn
	c.client = pb.NewStreamerServiceClient(conn)

	c.logger.Info("Connected to Streamer", "addr", config.StreamerAddr)
	return c, nil
}

// Stream implements the Service interface.
// Returns a bidirectional stream that wraps the gRPC stream.
func (c *streamerClient) Stream(ctx context.Context) (Stream, error) {
	grpcStream, err := c.client.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to establish stream: %w", err)
	}

	return &remoteStream{
		ctx:        ctx,
		grpcStream: grpcStream,
		logger:     c.logger,
	}, nil
}

// Close closes the client connection.
func (c *streamerClient) Close() error {
	c.cancel()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Compile-time check
var _ Service = (*streamerClient)(nil)

// --- remoteStream Implementation ---

// remoteStream implements the Stream interface for remote gRPC communication.
type remoteStream struct {
	ctx        context.Context
	grpcStream pb.StreamerService_StreamClient
	logger     *slog.Logger

	closedMu sync.Mutex
	closed   bool

	// Pending subscribe requests waiting for responses
	pendingSubscribes   map[string]chan *pb.SubscribeResponse
	pendingSubscribesMu sync.Mutex

	// recvLoop runs in background to route event deliveries
	recvChan chan *EventDelivery
	recvErr  error
	recvOnce sync.Once
}

// Subscribe creates a new subscription and returns the subscription ID.
func (rs *remoteStream) Subscribe(tenant, collection string, filters []Filter) (string, error) {
	rs.ensureRecvLoop()

	rs.closedMu.Lock()
	if rs.closed {
		rs.closedMu.Unlock()
		return "", io.EOF
	}
	rs.closedMu.Unlock()

	// Generate subscription ID
	subID := uuid.New().String()

	// Create response channel
	respChan := make(chan *pb.SubscribeResponse, 1)
	rs.pendingSubscribesMu.Lock()
	rs.pendingSubscribes[subID] = respChan
	rs.pendingSubscribesMu.Unlock()

	defer func() {
		rs.pendingSubscribesMu.Lock()
		delete(rs.pendingSubscribes, subID)
		rs.pendingSubscribesMu.Unlock()
	}()

	// Send subscribe request - directly construct proto
	protoReq := &pb.SubscribeRequest{
		SubscriptionId: subID,
		Tenant:         tenant,
		Collection:     collection,
		Filters:        filtersToProto(filters),
	}
	if err := rs.grpcStream.Send(&pb.GatewayMessage{
		Payload: &pb.GatewayMessage_Subscribe{Subscribe: protoReq},
	}); err != nil {
		return "", err
	}

	// Wait for response
	select {
	case resp := <-respChan:
		if !resp.Success {
			return "", fmt.Errorf("subscribe failed: %s", resp.Error)
		}
		return resp.SubscriptionId, nil
	case <-rs.ctx.Done():
		return "", rs.ctx.Err()
	}
}

// Unsubscribe removes a subscription by ID.
func (rs *remoteStream) Unsubscribe(subscriptionID string) error {
	rs.closedMu.Lock()
	if rs.closed {
		rs.closedMu.Unlock()
		return io.EOF
	}
	rs.closedMu.Unlock()

	// Send unsubscribe request - directly construct proto
	return rs.grpcStream.Send(&pb.GatewayMessage{
		Payload: &pb.GatewayMessage_Unsubscribe{
			Unsubscribe: &pb.UnsubscribeRequest{
				SubscriptionId: subscriptionID,
			},
		},
	})
}

// Recv receives an EventDelivery from the remote Streamer.
func (rs *remoteStream) Recv() (*EventDelivery, error) {
	rs.ensureRecvLoop()

	select {
	case delivery, ok := <-rs.recvChan:
		if !ok {
			if rs.recvErr != nil {
				return nil, rs.recvErr
			}
			return nil, io.EOF
		}
		return delivery, nil
	case <-rs.ctx.Done():
		return nil, rs.ctx.Err()
	}
}

// Close closes the stream.
func (rs *remoteStream) Close() error {
	rs.closedMu.Lock()
	defer rs.closedMu.Unlock()

	if !rs.closed {
		rs.closed = true
		return rs.grpcStream.CloseSend()
	}
	return nil
}

// ensureRecvLoop starts the background receive loop if not already running.
func (rs *remoteStream) ensureRecvLoop() {
	rs.recvOnce.Do(func() {
		rs.recvChan = make(chan *EventDelivery, 100)
		rs.pendingSubscribes = make(map[string]chan *pb.SubscribeResponse)
		go rs.recvLoop()
	})
}

// recvLoop reads from grpcStream and routes messages directly.
func (rs *remoteStream) recvLoop() {
	defer close(rs.recvChan)
	for {
		protoMsg, err := rs.grpcStream.Recv()
		if err != nil {
			rs.recvErr = err
			return
		}
		// Process proto directly and route appropriately
		switch payload := protoMsg.Payload.(type) {
		case *pb.StreamerMessage_Delivery:
			delivery := protoToEventDelivery(payload.Delivery)
			select {
			case rs.recvChan <- delivery:
			case <-rs.ctx.Done():
				return
			}
		case *pb.StreamerMessage_SubscribeResponse:
			rs.handleSubscribeResponse(payload.SubscribeResponse)
		case *pb.StreamerMessage_HeartbeatAck:
			// Ignore heartbeat acks
		}
	}
}

// handleSubscribeResponse routes a subscribe response to the waiting caller.
func (rs *remoteStream) handleSubscribeResponse(resp *pb.SubscribeResponse) {
	rs.pendingSubscribesMu.Lock()
	if ch, ok := rs.pendingSubscribes[resp.SubscriptionId]; ok {
		select {
		case ch <- resp:
		default:
		}
	}
	rs.pendingSubscribesMu.Unlock()
}

// Compile-time check
var _ Stream = (*remoteStream)(nil)
