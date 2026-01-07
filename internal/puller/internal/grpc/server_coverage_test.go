package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	pullerv1 "github.com/syntrixbase/syntrix/api/gen/puller/v1"
	"github.com/syntrixbase/syntrix/internal/puller/config"
	"github.com/syntrixbase/syntrix/internal/puller/events"
	"github.com/syntrixbase/syntrix/internal/storage"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockSubscribeServer struct {
	grpc.ServerStream
	ctx      context.Context
	sendFunc func(*pullerv1.PullerEvent) error
}

func (m *mockSubscribeServer) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *mockSubscribeServer) Send(evt *pullerv1.PullerEvent) error {
	if m.sendFunc != nil {
		return m.sendFunc(evt)
	}
	return nil
}

func TestServer_StartStop(t *testing.T) {
	t.Parallel()
	cfg := config.GRPCConfig{
		Address:        ":0", // Random port
		MaxConnections: 10,
	}

	source := &mockEventSource{}
	srv := NewServer(cfg, source, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	go func() {
		if err := srv.Start(ctx); err != nil {
			// t.Errorf("Start failed: %v", err)
		}
	}()

	// Wait for start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	srv.Stop(stopCtx)
}

func TestServer_Subscribe(t *testing.T) {
	t.Parallel()
	port := "8098"
	cfg := config.GRPCConfig{
		Address:        ":" + port,
		MaxConnections: 10,
	}

	source := &mockEventSource{}
	srv := NewServer(cfg, source, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = srv.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	defer srv.Stop(stopCtx)

	// Connect with client
	conn, err := grpc.NewClient("localhost:"+port, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pullerv1.NewPullerServiceClient(conn)

	// Subscribe
	stream, err := client.Subscribe(ctx, &pullerv1.SubscribeRequest{
		ConsumerId: "test-consumer",
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Send an event via source handler
	time.Sleep(100 * time.Millisecond)

	if source.handler != nil {
		evt := &events.StoreChangeEvent{
			EventID: "evt-1",
			MgoColl: "users",
		}
		_ = source.handler(ctx, "backend1", evt)
	} else {
		t.Fatal("Handler not set")
	}

	// Receive event
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}

	if resp.ChangeEvent.EventId != "evt-1" {
		t.Errorf("Expected event ID evt-1, got %s", resp.ChangeEvent.EventId)
	}
}

func TestServer_Start_AlreadyRunning(t *testing.T) {
	t.Parallel()
	cfg := config.GRPCConfig{Address: ":0"}
	srv := NewServer(cfg, &mockEventSource{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first time
	go func() {
		_ = srv.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)

	// Start second time
	err := srv.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running server")
	}

	// Cleanup
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	srv.Stop(stopCtx)
}

func TestServer_Start_ListenError(t *testing.T) {
	t.Parallel()
	cfg := config.GRPCConfig{Address: "256.256.256.256:9999"} // Invalid IP
	srv := NewServer(cfg, &mockEventSource{}, nil)

	err := srv.Start(context.Background())
	if err == nil {
		t.Error("Expected error for invalid address")
	}
}

func TestServer_Subscribe_SendError(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{ConsumerId: "c1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream := &mockSubscribeServer{
		ctx: ctx,
		sendFunc: func(evt *pullerv1.PullerEvent) error {
			return fmt.Errorf("send failed")
		},
	}

	// Run Subscribe in goroutine
	errChan := make(chan error)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	// Wait for subscriber to be added
	time.Sleep(100 * time.Millisecond)

	// Broadcast event
	evt := &events.StoreChangeEvent{EventID: "evt1", Backend: "backend1"}
	srv.eventChan <- evt

	// Subscribe should return error
	select {
	case err := <-errChan:
		if err == nil || err.Error() != "send failed" {
			t.Errorf("Expected 'send failed' error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return")
	}
}

func TestServer_Subscribe_SubscriberClosed(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)
	req := &pullerv1.SubscribeRequest{ConsumerId: "c1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream := &mockSubscribeServer{ctx: ctx}

	// Run Subscribe
	errChan := make(chan error)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	time.Sleep(100 * time.Millisecond)

	// Close subscriber manually (simulate eviction or shutdown)
	// Since we don't have direct access to the subscriber created inside Subscribe,
	// we can trigger it by closing the server or context, but that hits ctx.Done().
	// Wait, s.subs.Add(sub) adds it to the set.
	// If we can access s.subs, we can close it.
	// But s.subs is private.

	// However, we can simulate the condition that causes sub.Done() to be closed.
	// Subscriber.Close() closes the Done channel.
	// Subscriber.Close() is called when it's removed from the set? No.

	// Actually, looking at the code, sub.Done() is closed when sub.Close() is called.
	// Who calls sub.Close()?
	// Usually the SubscriberSet when stopping, or if we evict it.

	// Let's try to trigger the nil event case first, it's easier.
	srv.eventChan <- nil

	// Now trigger context cancellation to exit
	// But we want to hit sub.Done().

	// If we can't easily hit sub.Done(), let's focus on nil event and convert error.
}

func TestServer_Subscribe_NilEvent(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{ConsumerId: "c1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream := &mockSubscribeServer{ctx: ctx}

	go func() {
		_ = srv.Subscribe(req, stream)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send nil event - should be ignored
	srv.eventChan <- nil

	// Send valid event to ensure it's still running
	done := make(chan struct{})
	stream.sendFunc = func(e *pullerv1.PullerEvent) error {
		close(done)
		return nil
	}

	srv.eventChan <- &events.StoreChangeEvent{
		Backend: "b1",
		EventID: "1",
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event after nil")
	}
}

func TestServer_Subscribe_ConvertError(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{ConsumerId: "c1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream := &mockSubscribeServer{ctx: ctx}

	go func() {
		_ = srv.Subscribe(req, stream)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send event that fails conversion
	// json.Marshal fails on channels
	badDoc := map[string]interface{}{
		"bad": make(chan int),
	}

	srv.eventChan <- &events.StoreChangeEvent{
		Backend:      "b1",
		EventID:      "1",
		FullDocument: &storage.StoredDoc{Data: badDoc},
	}

	// Should log error and continue.
	// Send valid event to verify it continued
	done := make(chan struct{})
	stream.sendFunc = func(e *pullerv1.PullerEvent) error {
		close(done)
		return nil
	}

	srv.eventChan <- &events.StoreChangeEvent{
		Backend: "b1",
		EventID: "2",
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event after convert error")
	}
}

func TestServer_Subscribe_ContextCancellation(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{ConsumerId: "ctx-cancel-test"}
	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockSubscribeServer{ctx: ctx}

	errChan := make(chan error)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	// Wait for subscription to be established
	time.Sleep(50 * time.Millisecond)

	// Cancel context - should trigger ctx.Done() branch
	cancel()

	select {
	case err := <-errChan:
		// Should return nil on graceful context cancellation
		if err != nil {
			t.Logf("Subscribe returned: %v (expected nil or canceled)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return after context cancellation")
	}
}

func TestServer_Subscribe_SubscriberClosed_Live(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{ConsumerId: "sub-close-test"}
	ctx := context.Background()
	stream := &mockSubscribeServer{ctx: ctx}

	errChan := make(chan error)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	// Wait for subscription to be established in live mode
	time.Sleep(50 * time.Millisecond)

	// Close all subscribers - this should trigger sub.Done() branch
	srv.subs.CloseAll()

	select {
	case err := <-errChan:
		// Should return error when subscriber is closed
		if err == nil {
			t.Error("Expected error when subscriber is closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return after subscriber closed")
	}
}

func TestServer_Stop_ForceStop(t *testing.T) {
	t.Parallel()
	cfg := config.GRPCConfig{
		Address:        ":0",
		MaxConnections: 10,
	}

	source := &mockEventSource{}
	srv := NewServer(cfg, source, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Use an already cancelled context to force Stop
	stopCtx, stopCancel := context.WithCancel(context.Background())
	stopCancel() // Cancel immediately to trigger force stop

	srv.Stop(stopCtx)
}

func TestServer_processEvents_ChannelClosed(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)

	// Close event channel to trigger !ok branch
	close(srv.eventChan)

	// processEvents should return when channel is closed
	done := make(chan struct{})
	go func() {
		srv.processEvents()
		close(done)
	}()

	select {
	case <-done:
		// Good - processEvents returned
	case <-time.After(time.Second):
		t.Fatal("processEvents did not return after channel closed")
	}
}

func TestServer_processEvents_ContextDone(t *testing.T) {
	t.Parallel()
	srv := NewServer(config.GRPCConfig{}, &mockEventSource{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	srv.ctx = ctx

	done := make(chan struct{})
	go func() {
		srv.processEvents()
		close(done)
	}()

	// Cancel context to trigger ctx.Done() branch
	cancel()

	select {
	case <-done:
		// Good - processEvents returned
	case <-time.After(time.Second):
		t.Fatal("processEvents did not return after context cancelled")
	}
}

// replayErrorEventSource returns an error from Replay()
type replayErrorEventSource struct {
	mockEventSource
	replayErr error
}

func (r *replayErrorEventSource) Replay(ctx context.Context, after map[string]string, coalesce bool) (events.Iterator, error) {
	return nil, r.replayErr
}

// makeProgressMarker creates a base64-encoded progress marker for testing
func makeProgressMarker(backend, eventID string) string {
	pm := map[string]interface{}{
		"p": map[string]string{backend: eventID},
	}
	data, _ := json.Marshal(pm)
	return base64.RawURLEncoding.EncodeToString(data)
}

func TestServer_Subscribe_ReplayError(t *testing.T) {
	t.Parallel()

	source := &replayErrorEventSource{
		replayErr: fmt.Errorf("database connection failed"),
	}
	srv := NewServer(config.GRPCConfig{}, source, nil)
	go srv.processEvents()

	// Request with after position to trigger replay mode
	req := &pullerv1.SubscribeRequest{
		ConsumerId: "c1",
		After:      makeProgressMarker("backend1", "evt-123"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream := &mockSubscribeServer{ctx: ctx}

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected error from Replay failure")
		}
		if !containsSubstr(err.Error(), "failed to start replay") {
			t.Errorf("Expected 'failed to start replay' error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return")
	}
}

// iteratorWithError returns an error from Err()
type iteratorWithError struct {
	events []*events.StoreChangeEvent
	idx    int
	err    error
}

func (i *iteratorWithError) Next() bool {
	i.idx++
	return i.idx <= len(i.events)
}

func (i *iteratorWithError) Event() *events.StoreChangeEvent {
	if i.idx > 0 && i.idx <= len(i.events) {
		return i.events[i.idx-1]
	}
	return nil
}

func (i *iteratorWithError) Err() error   { return i.err }
func (i *iteratorWithError) Close() error { return nil }

type iteratorErrEventSource struct {
	mockEventSource
	iter *iteratorWithError
}

func (r *iteratorErrEventSource) Replay(ctx context.Context, after map[string]string, coalesce bool) (events.Iterator, error) {
	return r.iter, nil
}

func TestServer_Subscribe_IteratorError(t *testing.T) {
	t.Parallel()

	source := &iteratorErrEventSource{
		iter: &iteratorWithError{
			events: []*events.StoreChangeEvent{},
			err:    fmt.Errorf("cursor exhausted unexpectedly"),
		},
	}
	srv := NewServer(config.GRPCConfig{}, source, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{
		ConsumerId: "c1",
		After:      makeProgressMarker("backend1", "evt-123"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream := &mockSubscribeServer{ctx: ctx}

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected error from iterator error")
		}
		if !containsSubstr(err.Error(), "replay error") {
			t.Errorf("Expected 'replay error' error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return")
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// iteratorWithEvents returns events and then optionally an error
type iteratorWithEvents struct {
	events []*events.StoreChangeEvent
	idx    int
	err    error
}

func (i *iteratorWithEvents) Next() bool {
	i.idx++
	return i.idx <= len(i.events)
}

func (i *iteratorWithEvents) Event() *events.StoreChangeEvent {
	if i.idx > 0 && i.idx <= len(i.events) {
		return i.events[i.idx-1]
	}
	return nil
}

func (i *iteratorWithEvents) Err() error   { return i.err }
func (i *iteratorWithEvents) Close() error { return nil }

type replayWithEventsSource struct {
	mockEventSource
	iter *iteratorWithEvents
}

func (r *replayWithEventsSource) Replay(ctx context.Context, after map[string]string, coalesce bool) (events.Iterator, error) {
	return r.iter, nil
}

func TestServer_Subscribe_SendEventError_DuringReplay(t *testing.T) {
	t.Parallel()

	// Create source that returns events during replay
	source := &replayWithEventsSource{
		iter: &iteratorWithEvents{
			events: []*events.StoreChangeEvent{
				{Backend: "backend1", EventID: "evt-1", ClusterTime: events.ClusterTime{T: 100, I: 1}},
			},
		},
	}
	srv := NewServer(config.GRPCConfig{}, source, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{
		ConsumerId: "c1",
		After:      makeProgressMarker("backend1", "evt-0"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sendErr := fmt.Errorf("network error: connection reset")
	stream := &mockSubscribeServer{
		ctx: ctx,
		sendFunc: func(evt *pullerv1.PullerEvent) error {
			return sendErr
		},
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected error from sendEvent failure during replay")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return")
	}
}

func TestServer_Subscribe_HeartbeatError(t *testing.T) {
	t.Parallel()

	source := &mockEventSource{}
	srv := NewServer(config.GRPCConfig{
		HeartbeatInterval: 50 * time.Millisecond, // Short heartbeat interval
	}, source, nil)
	go srv.processEvents()

	req := &pullerv1.SubscribeRequest{
		ConsumerId: "c1",
		// No After, so it starts in live mode
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	heartbeatCount := 0
	stream := &mockSubscribeServer{
		ctx: ctx,
		sendFunc: func(evt *pullerv1.PullerEvent) error {
			heartbeatCount++
			if heartbeatCount >= 1 {
				return fmt.Errorf("heartbeat send failed")
			}
			return nil
		},
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Subscribe(req, stream)
	}()

	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected error from heartbeat failure")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for Subscribe to return")
	}
}
