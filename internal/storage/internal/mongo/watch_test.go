package mongo

import (
	"context"
	"testing"
	"time"

	"github.com/syntrixbase/syntrix/internal/storage/types"
	"github.com/syntrixbase/syntrix/pkg/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoBackend_Watch(t *testing.T) {
	backend := setupTestBackend(t)
	defer backend.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tenant := "default"

	// Start Watching
	stream, err := backend.Watch(ctx, tenant, "users", nil, types.WatchOptions{})
	if err != nil {
		t.Fatalf("Watch failed (likely no replica set): %v", err)
		return
	}

	// Channel to collect errors from goroutine
	errChan := make(chan error, 3)

	// Perform Operations
	go func() {
		time.Sleep(50 * time.Millisecond) // Wait for watch to establish

		// Create
		doc := types.NewStoredDoc(tenant, "users", "watcher", map[string]interface{}{"msg": "hello"})
		if err := backend.Create(context.Background(), tenant, doc); err != nil {
			errChan <- err
			return
		}

		// Update with version filter (optimistic locking test)
		// doc.Version is int64(1) after NewStoredDoc
		filters := model.Filters{
			{Field: "version", Op: "==", Value: doc.Version}, // int64(1)
		}
		if err := backend.Update(context.Background(), tenant, "users/watcher", map[string]interface{}{"msg": "world"}, filters); err != nil {
			errChan <- err
			return
		}

		// Delete
		if err := backend.Delete(context.Background(), tenant, "users/watcher", nil); err != nil {
			errChan <- err
			return
		}
	}()

	// Verify Events
	expectedEvents := []types.EventType{types.EventCreate, types.EventUpdate, types.EventDelete}
	for i, expectedType := range expectedEvents {
		select {
		case evt := <-stream:
			t.Logf("Received event: Type=%s ID=%s", evt.Type, evt.Id)
			require.Equal(t, expectedType, evt.Type, "Event %d type mismatch", i)
			if expectedType == types.EventCreate {
				require.NotNil(t, evt.Document)
				assert.Equal(t, "hello", evt.Document.Data["msg"])
			} else if expectedType == types.EventUpdate {
				require.NotNil(t, evt.Document)
				assert.Equal(t, "world", evt.Document.Data["msg"])
			}
			// Delete event may not have Document
		case err := <-errChan:
			t.Fatalf("Operation failed: %v", err)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for event %s", expectedType)
		}
	}
}

func TestMongoBackend_Watch_Recreate(t *testing.T) {
	backend := setupTestBackend(t)
	defer backend.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tenant := "default"

	stream, err := backend.Watch(ctx, tenant, "users", nil, types.WatchOptions{})
	if err != nil {
		t.Fatalf("Watch recreate test failed (likely no replica set): %v", err)
		return
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		doc := types.NewStoredDoc(tenant, "users", "recreate", map[string]interface{}{"msg": "v1"})
		_ = backend.Create(context.Background(), tenant, doc)

		_ = backend.Delete(context.Background(), tenant, "users/recreate", nil)

		_ = backend.Create(context.Background(), tenant, types.NewStoredDoc(tenant, "users", "recreate", map[string]interface{}{"msg": "v2"}))
	}()

	expected := []types.EventType{types.EventCreate, types.EventDelete, types.EventCreate}
	msgs := []string{"v1", "v2"}
	createIdx := 0
	for _, evtType := range expected {
		select {
		case evt := <-stream:
			assert.Equal(t, evtType, evt.Type)
			if evtType == types.EventCreate {
				if assert.NotNil(t, evt.Document) && createIdx < len(msgs) {
					assert.Equal(t, msgs[createIdx], evt.Document.Data["msg"])
				}
				createIdx++
			}
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for event %s", evtType)
		}
	}
}

func TestMongoBackend_Watch_Recreate_WithBefore(t *testing.T) {
	backend := setupTestBackend(t)
	defer backend.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tenant := "default"

	stream, err := backend.Watch(ctx, tenant, "users", nil, types.WatchOptions{IncludeBefore: true})
	if err != nil {
		t.Fatalf("Watch recreate (before) test failed (likely no replica set): %v", err)
		return
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		doc := types.NewStoredDoc(tenant, "users", "recreate-before", map[string]interface{}{"msg": "v1"})
		_ = backend.Create(context.Background(), tenant, doc)

		_ = backend.Delete(context.Background(), tenant, "users/recreate-before", nil)

		_ = backend.Create(context.Background(), tenant, types.NewStoredDoc(tenant, "users", "recreate-before", map[string]interface{}{"msg": "v2"}))
	}()

	expected := []types.EventType{types.EventCreate, types.EventDelete, types.EventCreate}
	msgs := []string{"v1", "v2"}
	createIdx := 0
	for _, evtType := range expected {
		select {
		case evt := <-stream:
			assert.Equal(t, evtType, evt.Type)
			if evtType == types.EventCreate {
				if assert.NotNil(t, evt.Document) && createIdx < len(msgs) {
					assert.Equal(t, msgs[createIdx], evt.Document.Data["msg"])
				}
				assert.Nil(t, evt.Before)
				createIdx++
			}
			if evtType == types.EventDelete {
				if evt.Before != nil {
					assert.Equal(t, "v1", evt.Before.Data["msg"])
				}
			}
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for event %s", evtType)
		}
	}
}
