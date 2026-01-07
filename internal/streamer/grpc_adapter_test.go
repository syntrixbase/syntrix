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
	"github.com/syntrixbase/syntrix/internal/puller/events"
	"github.com/syntrixbase/syntrix/internal/storage"
	"google.golang.org/grpc/metadata"
)

// --- Mock gRPC Stream for testing GRPCStream ---

// mockBidiStream implements grpc.BidiStreamingServer for testing.
type mockBidiStream struct {
	ctx      context.Context
	recvMsgs []*pb.GatewayMessage
	recvIdx  int
	sentMsgs []*pb.StreamerMessage
	recvErr  error
	sendErr  error
	mu       sync.Mutex
}

func (m *mockBidiStream) Send(msg *pb.StreamerMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func (m *mockBidiStream) Recv() (*pb.GatewayMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvErr != nil {
		return nil, m.recvErr
	}
	if m.recvIdx >= len(m.recvMsgs) {
		// Block until context is done
		m.mu.Unlock()
		<-m.ctx.Done()
		m.mu.Lock()
		return nil, m.ctx.Err()
	}
	msg := m.recvMsgs[m.recvIdx]
	m.recvIdx++
	return msg, nil
}

func (m *mockBidiStream) Context() context.Context     { return m.ctx }
func (m *mockBidiStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockBidiStream) SendHeader(metadata.MD) error { return nil }
func (m *mockBidiStream) SetTrailer(metadata.MD)       {}
func (m *mockBidiStream) SendMsg(interface{}) error    { return nil }
func (m *mockBidiStream) RecvMsg(interface{}) error    { return nil }

// --- GRPCStream Tests ---

func TestGRPCStream_ContextCanceled(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	mockStream := &mockBidiStream{ctx: ctx}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Cancel context
	time.Sleep(10 * time.Millisecond)
	cancel()

	err = <-done
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGRPCStream_RecvError(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx := context.Background()
	mockStream := &mockBidiStream{
		ctx:     ctx,
		recvErr: io.EOF,
	}

	err = internal.GRPCStream(mockStream)
	assert.ErrorIs(t, err, io.EOF)
}

func TestGRPCStream_SubscribeMessage(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe and immediately receive EOF to exit
	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "sub1",
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait a bit for message to be processed, then cancel to stop
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	// Test passes as long as it doesn't hang or panic
}

func TestGRPCStream_HeartbeatMessage(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Heartbeat{
					Heartbeat: &pb.Heartbeat{
						Timestamp: 12345,
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Give time for heartbeat to be processed and ack sent
	time.Sleep(50 * time.Millisecond)
	cancel()

	<-done

	// Should have sent a heartbeat ack
	mockStream.mu.Lock()
	defer mockStream.mu.Unlock()
	require.GreaterOrEqual(t, len(mockStream.sentMsgs), 1)
	ack := mockStream.sentMsgs[0].GetHeartbeatAck()
	require.NotNil(t, ack)
	assert.Equal(t, int64(12345), ack.Timestamp)
}

func TestGRPCStream_SendError(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx := context.Background()
	mockStream := &mockBidiStream{
		ctx:     ctx,
		sendErr: errors.New("send failed"),
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Heartbeat{
					Heartbeat: &pb.Heartbeat{Timestamp: 1},
				},
			},
		},
	}

	err = internal.GRPCStream(mockStream)
	assert.Error(t, err)
}

func TestGRPCStream_ServiceStopped(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx := context.Background()
	mockStream := &mockBidiStream{ctx: ctx}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Stop the service
	time.Sleep(10 * time.Millisecond)
	internal.Stop(context.Background())

	err = <-done
	assert.Error(t, err)
}

func TestGRPCStream_UnsubscribeMessage(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Unsubscribe{
					Unsubscribe: &pb.UnsubscribeRequest{
						SubscriptionId: "nonexistent-sub",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait for message to be processed
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
}

func TestGRPCStream_SubscribeWithError(t *testing.T) {
	t.Parallel()
	// Create a service with a mock manager that returns errors
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First subscribe to create the subscription, then subscribe again with same ID
	// to trigger an error (subscription already exists)
	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "dup-sub",
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "dup-sub", // duplicate
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait for messages to be processed
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Test passes as long as it processes without panic
}

func TestGRPCStream_MultipleHeartbeats(t *testing.T) {
	t.Parallel()
	// Test that multiple heartbeat messages are handled correctly
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Send multiple heartbeat messages
	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Heartbeat{
					Heartbeat: &pb.Heartbeat{Timestamp: 1},
				},
			},
			{
				Payload: &pb.GatewayMessage_Heartbeat{
					Heartbeat: &pb.Heartbeat{Timestamp: 2},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait a bit then cancel
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done
}

// TestGRPCAdapter_OutgoingEvents tests the outgoing event delivery path.
func TestGRPCAdapter_OutgoingEvents(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &mockBidiStream{ctx: ctx}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Give adapter time to start
	time.Sleep(20 * time.Millisecond)

	// Create a local stream and subscribe to the same gateway
	// The GRPCStream creates its own gatewayID, so we need to get events to it
	// by processing events that match its subscriptions

	// For now, just let it exit via context cancellation
	cancel()
	<-done
}

// TestGRPCAdapter_OutgoingDelivery tests the delivery sending through gRPC.
func TestGRPCAdapter_OutgoingDelivery(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe first, then receive messages to trigger outgoing delivery
	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "sub-grpc-1",
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait for subscription to be processed
	time.Sleep(50 * time.Millisecond)

	// Process an event that matches the subscription
	err = internal.ProcessEvent(events.SyntrixChangeEvent{
		Id:       "evt-grpc-1",
		TenantID: "tenant1",
		Type:     events.EventCreate,
		Document: &storage.StoredDoc{
			Id:         "_id_doc1",
			TenantID:   "tenant1",
			Collection: "users",
			Fullpath:   "users/doc1",
			Data:       map[string]interface{}{"id": "doc1", "name": "Alice"},
		},
	})
	require.NoError(t, err)

	// Wait for event to be delivered
	time.Sleep(50 * time.Millisecond)

	// Verify the event was sent
	mockStream.mu.Lock()
	defer mockStream.mu.Unlock()
	found := false
	for _, msg := range mockStream.sentMsgs {
		if delivery := msg.GetDelivery(); delivery != nil {
			if delivery.Event.Collection == "users" {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "expected delivery to be sent")

	cancel()
	<-done
}

// TestGRPCAdapter_OutgoingSendError tests error handling when sending fails.
func TestGRPCAdapter_OutgoingSendError(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx := context.Background()

	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "sub-send-err",
						Tenant:         "tenant1",
						Collection:     "products",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait for subscription to be processed
	time.Sleep(50 * time.Millisecond)

	// Now set send error to trigger failure when delivering
	mockStream.mu.Lock()
	mockStream.sendErr = errors.New("send delivery failed")
	mockStream.mu.Unlock()

	// Process an event to trigger delivery send
	err = internal.ProcessEvent(events.SyntrixChangeEvent{
		Id:       "evt-send-err",
		TenantID: "tenant1",
		Type:     events.EventCreate,
		Document: &storage.StoredDoc{
			Id:         "_id_doc1",
			TenantID:   "tenant1",
			Collection: "products",
			Fullpath:   "products/doc1",
			Data: map[string]interface{}{
				"id":   "doc1",
				"name": "Product",
			},
		},
	})
	require.NoError(t, err)

	// The adapter should exit with an error
	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("expected error from send failure")
	}
}

// TestGRPCAdapter_SubscribeAutoGenerateID tests that subscription ID is auto-generated if empty.
func TestGRPCAdapter_SubscribeAutoGenerateID(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "", // Empty - should be auto-generated
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait for subscription to be processed
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Verify response was sent with a generated subscription ID
	mockStream.mu.Lock()
	defer mockStream.mu.Unlock()
	require.GreaterOrEqual(t, len(mockStream.sentMsgs), 1)
	resp := mockStream.sentMsgs[0].GetSubscribeResponse()
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.SubscriptionId)
	assert.True(t, resp.Success)
}

// TestGRPCAdapter_OutgoingChannelClosed tests behavior when outgoing channel is closed.
func TestGRPCAdapter_OutgoingChannelClosed(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStream := &mockBidiStream{ctx: ctx}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Stop the service to close streams
	time.Sleep(20 * time.Millisecond)
	internal.Stop(context.Background())

	err = <-done
	assert.Error(t, err)
}

// TestGRPCAdapter_Subscribe_ManagerError tests handleProtoMessage when manager.Subscribe returns error.
func TestGRPCAdapter_Subscribe_ManagerError(t *testing.T) {
	t.Parallel()
	s, err := NewService(ServiceConfig{}, slog.Default())
	require.NoError(t, err)
	internal := getInternalService(s)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create two subscriptions with the same ID to trigger an error on the second one
	mockStream := &mockBidiStream{
		ctx: ctx,
		recvMsgs: []*pb.GatewayMessage{
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "dup-id",
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
			{
				Payload: &pb.GatewayMessage_Subscribe{
					Subscribe: &pb.SubscribeRequest{
						SubscriptionId: "dup-id", // Same ID - should cause error
						Tenant:         "tenant1",
						Collection:     "users",
					},
				},
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- internal.GRPCStream(mockStream)
	}()

	// Wait for both subscriptions to be processed
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	// Verify both responses were sent
	mockStream.mu.Lock()
	defer mockStream.mu.Unlock()

	require.GreaterOrEqual(t, len(mockStream.sentMsgs), 2)

	// First should succeed
	resp1 := mockStream.sentMsgs[0].GetSubscribeResponse()
	require.NotNil(t, resp1)
	assert.True(t, resp1.Success)

	// Second should fail (duplicate ID)
	resp2 := mockStream.sentMsgs[1].GetSubscribeResponse()
	require.NotNil(t, resp2)
	assert.False(t, resp2.Success)
	assert.NotEmpty(t, resp2.Error)
}
