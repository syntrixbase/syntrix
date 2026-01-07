package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/internal/storage/types"
	"github.com/syntrixbase/syntrix/pkg/model"
)

func TestHub_Register_Closed(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)

	// Wait for hub to start (runCtx to be set)
	for i := 0; i < 100; i++ {
		if hub.Done() != nil {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Cancel context to stop hub
	cancel()

	// Wait for hub to close
	select {
	case <-hub.Done():
	case <-time.After(time.Second):
		t.Fatal("hub did not close in time")
	}

	client := &Client{
		hub:  hub,
		send: make(chan BaseMessage, 256),
	}

	// Register should return false
	success := hub.Register(client)
	assert.False(t, success, "Register should return false when hub is closed")
}

func TestHub_Unregister_Closed(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)

	// Wait for hub to start
	for i := 0; i < 100; i++ {
		if hub.Done() != nil {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Cancel context to stop hub
	cancel()

	// Wait for hub to close
	select {
	case <-hub.Done():
	case <-time.After(time.Second):
		t.Fatal("hub did not close in time")
	}

	client := &Client{
		hub:  hub,
		send: make(chan BaseMessage, 256),
	}

	// Unregister should not panic or block
	hub.Unregister(client)
}

func TestHub_Broadcast_Closed(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)

	// Wait for hub to start
	for i := 0; i < 100; i++ {
		if hub.Done() != nil {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Cancel context to stop hub
	cancel()

	// Wait for hub to close
	select {
	case <-hub.Done():
	case <-time.After(time.Second):
		t.Fatal("hub did not close in time")
	}

	// Broadcast should not panic or block
	hub.Broadcast(storage.Event{})
}

func TestHub_Broadcast_BackpressureFallback(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)

	// Wait for runCtx to be set
	require.Eventually(t, func() bool { return hub.Done() != nil }, time.Second, 10*time.Millisecond)

	client := &Client{
		hub:  hub,
		send: make(chan BaseMessage, 1),
		subscriptions: map[string]Subscription{
			"sub1": {
				Query:       model.Query{Collection: "rooms"},
				IncludeData: true,
			},
		},
		allowAllTenants: true,
	}

	// Fill the channel to force default branch in non-blocking send.
	client.send <- BaseMessage{Type: "primed"}

	require.True(t, hub.Register(client))

	evt := storage.Event{
		Type: types.EventUpdate,
		Id:   "rooms/doc1",
		Document: &storage.StoredDoc{
			Collection: "rooms",
			Data:       map[string]interface{}{"x": 1},
		},
	}

	hub.Broadcast(evt)

	// Wait for the fallback select (with time.After) to trigger while channel remains full.
	time.Sleep(100 * time.Millisecond)

	// No new message should be enqueued because channel stayed full and fallback timeout fired.
	assert.Equal(t, 1, len(client.send))

	cancel()
}

func TestHub_Broadcast_NilDocument_WithCelFilter(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Wait for runCtx to be set
	require.Eventually(t, func() bool { return hub.Done() != nil }, time.Second, 10*time.Millisecond)

	// Create a CEL filter program
	filters := []model.Filter{{Field: "status", Op: "==", Value: "active"}}
	prg, err := compileFiltersToCEL(filters)
	require.NoError(t, err)

	client := &Client{
		hub:  hub,
		send: make(chan BaseMessage, 10),
		subscriptions: map[string]Subscription{
			"sub1": {
				Query:       model.Query{Collection: "items"},
				IncludeData: true,
				CelProgram:  prg,
			},
		},
		allowAllTenants: true,
	}

	require.True(t, hub.Register(client))

	// Broadcast event with nil Document - should be skipped by CelProgram check
	hub.Broadcast(storage.Event{
		Type:     types.EventUpdate,
		Id:       "items/doc1",
		Document: nil, // This triggers the "if message.Document == nil { continue }" path
	})

	// No message should be received since Document is nil
	select {
	case msg := <-client.send:
		t.Fatalf("Should not receive message when Document is nil: %+v", msg)
	case <-time.After(100 * time.Millisecond):
		// OK - no message as expected
	}
}

func TestHub_Broadcast_CelEvalError(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Wait for runCtx to be set
	require.Eventually(t, func() bool { return hub.Done() != nil }, time.Second, 10*time.Millisecond)

	// Create a CEL filter that accesses a field that will cause an error
	// The error happens when trying to access a nested field that doesn't exist
	filters := []model.Filter{{Field: "nested.deep.value", Op: "==", Value: "test"}}
	prg, err := compileFiltersToCEL(filters)
	require.NoError(t, err)

	client := &Client{
		hub:  hub,
		send: make(chan BaseMessage, 10),
		subscriptions: map[string]Subscription{
			"sub1": {
				Query:       model.Query{Collection: "items"},
				IncludeData: true,
				CelProgram:  prg,
			},
		},
		allowAllTenants: true,
	}

	require.True(t, hub.Register(client))

	// Broadcast event with data that will cause CEL eval error
	hub.Broadcast(storage.Event{
		Type: types.EventUpdate,
		Id:   "items/doc1",
		Document: &storage.StoredDoc{
			Collection: "items",
			Data:       map[string]interface{}{"status": "active"}, // Missing nested.deep.value
		},
	})

	// No message should be received due to CEL eval error
	select {
	case msg := <-client.send:
		t.Fatalf("Should not receive message when CEL eval fails: %+v", msg)
	case <-time.After(100 * time.Millisecond):
		// OK - no message as expected
	}
}

func TestDetermineEventTenant(t *testing.T) {
	tests := []struct {
		name     string
		event    storage.Event
		expected string
	}{
		{
			name:     "From Event TenantID",
			event:    storage.Event{TenantID: "t1"},
			expected: "t1",
		},
		{
			name: "From Document TenantID",
			event: storage.Event{
				Document: &storage.StoredDoc{TenantID: "t2"},
			},
			expected: "t2",
		},
		{
			name: "From Before TenantID",
			event: storage.Event{
				Before: &storage.StoredDoc{TenantID: "t3"},
			},
			expected: "t3",
		},
		{
			name:     "Empty",
			event:    storage.Event{},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, determineEventTenant(tc.event))
		})
	}
}
