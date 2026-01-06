package streamer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pb "github.com/syntrixbase/syntrix/api/gen/streamer/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	assert.Equal(t, "localhost:50052", cfg.StreamerAddr)
	assert.NotZero(t, cfg.ReconnectInterval)
	assert.NotZero(t, cfg.HeartbeatInterval)
}

// mockGRPCStreamClient implements pb.StreamerService_StreamClient for testing.
type mockGRPCStreamClient struct {
	sentMsgs   []*pb.GatewayMessage
	recvMsgs   []*pb.StreamerMessage
	recvIdx    int
	recvErr    error
	sendErr    error
	closedSend bool
	mu         sync.Mutex
}

func (m *mockGRPCStreamClient) Send(msg *pb.GatewayMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func (m *mockGRPCStreamClient) Recv() (*pb.StreamerMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvErr != nil {
		return nil, m.recvErr
	}
	if m.recvIdx >= len(m.recvMsgs) {
		return nil, io.EOF
	}
	msg := m.recvMsgs[m.recvIdx]
	m.recvIdx++
	return msg, nil
}

func (m *mockGRPCStreamClient) CloseSend() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closedSend = true
	return nil
}

func (m *mockGRPCStreamClient) Header() (metadata.MD, error) { return nil, nil }
func (m *mockGRPCStreamClient) Trailer() metadata.MD         { return nil }
func (m *mockGRPCStreamClient) Context() context.Context     { return context.Background() }
func (m *mockGRPCStreamClient) SendMsg(interface{}) error    { return nil }
func (m *mockGRPCStreamClient) RecvMsg(interface{}) error    { return nil }

// mockStreamerServiceClient implements pb.StreamerServiceClient for testing.
type mockStreamerServiceClient struct {
	pb.StreamerServiceClient
	streamClient pb.StreamerService_StreamClient
	streamErr    error
}

func (m *mockStreamerServiceClient) Stream(ctx context.Context, opts ...grpc.CallOption) (pb.StreamerService_StreamClient, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return m.streamClient, nil
}

func TestStreamerClient_Stream_Success(t *testing.T) {
	// Create a client with mock service client
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event:           &pb.StreamerEvent{Collection: "users"},
					},
				},
			},
		},
	}

	c := &streamerClient{
		client: &mockStreamerServiceClient{streamClient: mockStream},
		logger: slog.Default(),
	}

	stream, err := c.Stream(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Verify it's a remoteStream
	rs, ok := stream.(*remoteStream)
	assert.True(t, ok)
	assert.NotNil(t, rs.grpcStream)
}

func TestRemoteStream_Subscribe_SendError(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		sendErr: errors.New("send failed"),
	}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	_, err := rs.Subscribe("tenant1", "users", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send failed")
}

func TestRemoteStream_Subscribe_Closed(t *testing.T) {
	mockStream := &mockGRPCStreamClient{}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
		closed:     true,
	}

	_, err := rs.Subscribe("tenant1", "users", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestRemoteStream_Subscribe_ContextCanceled(t *testing.T) {
	mockStream := &mockGRPCStreamClient{}
	ctx, cancel := context.WithCancel(context.Background())
	rs := &remoteStream{
		ctx:        ctx,
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	// Start subscribe in goroutine
	done := make(chan error, 1)
	go func() {
		_, err := rs.Subscribe("tenant1", "users", nil)
		done <- err
	}()

	// Cancel context after a short delay
	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-done
	require.Error(t, err)
}

func TestRemoteStream_Unsubscribe(t *testing.T) {
	mockStream := &mockGRPCStreamClient{}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	err := rs.Unsubscribe("sub123")
	require.NoError(t, err)

	assert.Len(t, mockStream.sentMsgs, 1)
	assert.NotNil(t, mockStream.sentMsgs[0].GetUnsubscribe())
	assert.Equal(t, "sub123", mockStream.sentMsgs[0].GetUnsubscribe().SubscriptionId)
}

func TestRemoteStream_Unsubscribe_Closed(t *testing.T) {
	mockStream := &mockGRPCStreamClient{}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
		closed:     true,
	}

	err := rs.Unsubscribe("sub123")
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestRemoteStream_Unsubscribe_SendError(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		sendErr: errors.New("send failed"),
	}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	err := rs.Unsubscribe("sub123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send failed")
}

func TestRemoteStream_Recv_EventDelivery(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event: &pb.StreamerEvent{
							EventId:    "evt1",
							Tenant:     "tenant1",
							Collection: "users",
							Operation:  pb.OperationType_OPERATION_TYPE_INSERT,
						},
					},
				},
			},
		},
	}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	delivery, err := rs.Recv()
	require.NoError(t, err)
	require.NotNil(t, delivery)
	assert.Equal(t, []string{"sub1"}, delivery.SubscriptionIDs)
	assert.Equal(t, "evt1", delivery.Event.EventID)
}

func TestRemoteStream_Recv_SkipsHeartbeatAck(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_HeartbeatAck{
					HeartbeatAck: &pb.HeartbeatAck{},
				},
			},
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event: &pb.StreamerEvent{
							EventId: "evt1",
						},
					},
				},
			},
		},
	}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	// Should skip heartbeat and return delivery
	delivery, err := rs.Recv()
	require.NoError(t, err)
	require.NotNil(t, delivery)
	assert.Equal(t, "evt1", delivery.Event.EventID)
}

func TestRemoteStream_Recv_EOF(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{},
	}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	_, err := rs.Recv()
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestRemoteStream_Recv_Error(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		recvErr: errors.New("recv failed"),
	}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	_, err := rs.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recv failed")
}

func TestRemoteStream_Recv_ContextCanceled(t *testing.T) {
	// Use a blocking mock stream
	mockStream := &mockGRPCStreamClient{}
	ctx, cancel := context.WithCancel(context.Background())
	rs := &remoteStream{
		ctx:        ctx,
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	done := make(chan error, 1)
	go func() {
		_, err := rs.Recv()
		done <- err
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-done
	require.Error(t, err)
}

func TestRemoteStream_Close(t *testing.T) {
	mockStream := &mockGRPCStreamClient{}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	err := rs.Close()
	require.NoError(t, err)
	assert.True(t, mockStream.closedSend)
}

func TestRemoteStream_Close_Idempotent(t *testing.T) {
	mockStream := &mockGRPCStreamClient{}
	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	err := rs.Close()
	require.NoError(t, err)

	// Second close should be no-op
	err = rs.Close()
	require.NoError(t, err)
}

func TestRemoteStream_HandleSubscribeResponse(t *testing.T) {
	rs := &remoteStream{
		ctx:               context.Background(),
		logger:            slog.Default(),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
	}

	// Create a pending subscribe
	respChan := make(chan *pb.SubscribeResponse, 1)
	rs.pendingSubscribes["sub1"] = respChan

	// Handle response
	rs.handleSubscribeResponse(&pb.SubscribeResponse{
		SubscriptionId: "sub1",
		Success:        true,
	})

	// Verify response was routed
	select {
	case resp := <-respChan:
		assert.True(t, resp.Success)
		assert.Equal(t, "sub1", resp.SubscriptionId)
	case <-time.After(time.Second):
		t.Fatal("expected response")
	}
}

func TestRemoteStream_HandleSubscribeResponse_NoPending(t *testing.T) {
	rs := &remoteStream{
		ctx:               context.Background(),
		logger:            slog.Default(),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
	}

	// Should not panic with no pending
	rs.handleSubscribeResponse(&pb.SubscribeResponse{
		SubscriptionId: "unknown",
		Success:        true,
	})
}

// --- NewClient and Stream Tests ---

func TestNewClient_Success(t *testing.T) {
	config := ClientConfig{
		StreamerAddr: "localhost:50099", // doesn't need to be running
	}

	client, err := NewClient(config, nil)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Cleanup
	err = client.(*streamerClient).Close()
	assert.NoError(t, err)
}

func TestNewClient_DefaultValues(t *testing.T) {
	config := ClientConfig{
		StreamerAddr: "localhost:50099",
		// Leave intervals at zero to test defaults
	}

	client, err := NewClient(config, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, client)

	c := client.(*streamerClient)
	assert.Equal(t, 5*time.Second, c.config.ReconnectInterval)
	assert.Equal(t, 30*time.Second, c.config.HeartbeatInterval)

	c.Close()
}

func TestStreamerClient_Close(t *testing.T) {
	config := ClientConfig{
		StreamerAddr: "localhost:50099",
	}

	client, err := NewClient(config, nil)
	require.NoError(t, err)

	c := client.(*streamerClient)

	// Close should work
	err = c.Close()
	assert.NoError(t, err)

	// Close again should also work (conn already closed)
	err = c.Close()
	assert.Error(t, err) // Already closed
}

func TestStreamerClient_Close_NilConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &streamerClient{
		ctx:    ctx,
		cancel: cancel,
		conn:   nil, // No connection
	}

	// Close should return nil when conn is nil
	err := c.Close()
	assert.NoError(t, err)
}

func TestStreamerClient_Stream_NoServer(t *testing.T) {
	config := ClientConfig{
		StreamerAddr: "localhost:50099", // no server running
	}

	client, err := NewClient(config, nil)
	require.NoError(t, err)
	defer client.(*streamerClient).Close()

	// Stream should fail because no server is listening
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = client.Stream(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to establish stream")
}

func TestRemoteStream_Success(t *testing.T) {
	// Create a remoteStream with mock gRPC stream
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event: &pb.StreamerEvent{
							Tenant:     "tenant1",
							Collection: "users",
							DocumentId: "doc1",
							Operation:  pb.OperationType_OPERATION_TYPE_INSERT,
						},
					},
				},
			},
		},
	}

	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	// Recv should return the delivery
	delivery, err := rs.Recv()
	require.NoError(t, err)
	assert.Equal(t, "users", delivery.Event.Collection)
	assert.Equal(t, "doc1", delivery.Event.DocumentID)
}

func TestRemoteStream_Recv_SkipsSubscribeResponse(t *testing.T) {
	// Send a SubscribeResponse followed by a Delivery
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_SubscribeResponse{
					SubscribeResponse: &pb.SubscribeResponse{
						SubscriptionId: "sub1",
						Success:        true,
					},
				},
			},
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event:           &pb.StreamerEvent{Collection: "orders"},
					},
				},
			},
		},
	}

	rs := &remoteStream{
		ctx:               context.Background(),
		grpcStream:        mockStream,
		logger:            slog.Default(),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
	}

	// Recv should skip SubscribeResponse and return the delivery
	delivery, err := rs.Recv()
	require.NoError(t, err)
	assert.Equal(t, "orders", delivery.Event.Collection)
}

func TestRemoteStream_Recv_SkipsHeartbeatAck_WithMock(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_HeartbeatAck{
					HeartbeatAck: &pb.HeartbeatAck{Timestamp: 12345},
				},
			},
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event:           &pb.StreamerEvent{Collection: "products"},
					},
				},
			},
		},
	}

	rs := &remoteStream{
		ctx:               context.Background(),
		grpcStream:        mockStream,
		logger:            slog.Default(),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
	}

	// Recv should skip HeartbeatAck and return the delivery
	delivery, err := rs.Recv()
	require.NoError(t, err)
	assert.Equal(t, "products", delivery.Event.Collection)
}

// TestRemoteStream_Recv_ChannelClosedWithError tests Recv when channel closes with a prior error.
func TestRemoteStream_Recv_ChannelClosedWithError(t *testing.T) {
	// Create a mock stream that will return an error
	mockStream := &mockGRPCStreamClient{
		recvErr: errors.New("previous recv error"),
	}

	rs := &remoteStream{
		ctx:        context.Background(),
		grpcStream: mockStream,
		logger:     slog.Default(),
	}

	// Call Recv - it will start the recv loop which will hit the error
	_, err := rs.Recv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "previous recv error")
}

// TestRemoteStream_Recv_ChannelClosedNoError tests Recv when channel closes without error (EOF).
func TestRemoteStream_Recv_ChannelClosedNoError(t *testing.T) {
	rs := &remoteStream{
		ctx:               context.Background(),
		grpcStream:        &mockGRPCStreamClient{},
		logger:            slog.Default(),
		recvChan:          make(chan *EventDelivery),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
		recvErr:           nil, // No prior error
	}

	// Close the channel
	close(rs.recvChan)

	// Should return io.EOF
	_, err := rs.Recv()
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

// TestRemoteStream_Subscribe_FailedResponse tests Subscribe when server returns failure.
func TestRemoteStream_Subscribe_FailedResponse(t *testing.T) {
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			// Server will respond with failure
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rs := &remoteStream{
		ctx:               ctx,
		grpcStream:        mockStream,
		logger:            slog.Default(),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
	}

	// Start a goroutine to inject the failure response
	go func() {
		time.Sleep(20 * time.Millisecond)
		// Find the pending subscription and send a failure response
		rs.pendingSubscribesMu.Lock()
		for subID, ch := range rs.pendingSubscribes {
			select {
			case ch <- &pb.SubscribeResponse{
				SubscriptionId: subID,
				Success:        false,
				Error:          "subscription denied",
			}:
			default:
			}
		}
		rs.pendingSubscribesMu.Unlock()
	}()

	_, err := rs.Subscribe("tenant1", "denied-collection", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscribe failed")
}

// TestRemoteStream_RecvLoop_DeliveryContextDone tests recv loop exits on context done.
func TestRemoteStream_RecvLoop_DeliveryContextDone(t *testing.T) {
	// Create a mock that blocks on recv
	mockStream := &mockGRPCStreamClient{
		recvMsgs: []*pb.StreamerMessage{
			{
				Payload: &pb.StreamerMessage_Delivery{
					Delivery: &pb.EventDelivery{
						SubscriptionIds: []string{"sub1"},
						Event:           &pb.StreamerEvent{Collection: "blocking"},
					},
				},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	rs := &remoteStream{
		ctx:               ctx,
		grpcStream:        mockStream,
		logger:            slog.Default(),
		pendingSubscribes: make(map[string]chan *pb.SubscribeResponse),
		recvChan:          make(chan *EventDelivery), // Unbuffered - will block
	}

	// Start recv loop
	go rs.recvLoop()

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Cancel context while delivery is blocked
	cancel()

	// Give recv loop time to exit
	time.Sleep(30 * time.Millisecond)
}

// blockingMockGRPCStreamClient blocks until context is cancelled
type blockingMockGRPCStreamClient struct {
	mockGRPCStreamClient
	ctx context.Context
}

func (m *blockingMockGRPCStreamClient) Recv() (*pb.StreamerMessage, error) {
	// Block until context is done
	<-m.ctx.Done()
	return nil, m.ctx.Err()
}

// TestRemoteStream_Recv_ContextDoneAfterLoop tests Recv when context is cancelled after recv loop starts.
func TestRemoteStream_Recv_ContextDoneAfterLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Use a blocking mock that won't return until context is cancelled
	blockingMock := &blockingMockGRPCStreamClient{ctx: ctx}

	rs := &remoteStream{
		ctx:        ctx,
		grpcStream: blockingMock,
		logger:     slog.Default(),
	}

	done := make(chan error, 1)
	go func() {
		_, err := rs.Recv()
		done <- err
	}()

	// Give recv loop time to start
	time.Sleep(20 * time.Millisecond)

	// Cancel context - this should cause Recv to return via ctx.Done() case
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Recv should have returned after context cancellation")
	}
}

// Note: Full client tests require a running gRPC server.
// Integration tests are in tests/integration/
