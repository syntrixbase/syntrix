package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/codetrek/syntrix/internal/storage"
	"github.com/stretchr/testify/assert"
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
		time.Sleep(10 * time.Millisecond)
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
		time.Sleep(10 * time.Millisecond)
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
		time.Sleep(10 * time.Millisecond)
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
				Document: &storage.Document{TenantID: "t2"},
			},
			expected: "t2",
		},
		{
			name: "From Before TenantID",
			event: storage.Event{
				Before: &storage.Document{TenantID: "t3"},
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
