package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"

	"github.com/stretchr/testify/assert"
)

type mockQueryWatchError struct{}

func (m *mockQueryWatchError) GetDocument(ctx context.Context, tenant string, path string) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchError) CreateDocument(ctx context.Context, tenant string, doc model.Document) error {
	return nil
}
func (m *mockQueryWatchError) ReplaceDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchError) PatchDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchError) DeleteDocument(ctx context.Context, tenant string, path string, pred model.Filters) error {
	return nil
}
func (m *mockQueryWatchError) ExecuteQuery(ctx context.Context, tenant string, q model.Query) ([]model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchError) WatchCollection(ctx context.Context, tenant string, collection string) (<-chan storage.Event, error) {
	return nil, assert.AnError
}
func (m *mockQueryWatchError) Pull(ctx context.Context, tenant string, req storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error) {
	return nil, nil
}
func (m *mockQueryWatchError) Push(ctx context.Context, tenant string, req storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error) {
	return nil, nil
}

type mockQueryWatchStream struct {
	stream chan storage.Event
}

func (m *mockQueryWatchStream) GetDocument(ctx context.Context, tenant string, path string) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchStream) CreateDocument(ctx context.Context, tenant string, doc model.Document) error {
	return nil
}
func (m *mockQueryWatchStream) ReplaceDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchStream) PatchDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchStream) DeleteDocument(ctx context.Context, tenant string, path string, pred model.Filters) error {
	return nil
}
func (m *mockQueryWatchStream) ExecuteQuery(ctx context.Context, tenant string, q model.Query) ([]model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchStream) WatchCollection(ctx context.Context, tenant string, collection string) (<-chan storage.Event, error) {
	return m.stream, nil
}
func (m *mockQueryWatchStream) Pull(ctx context.Context, tenant string, req storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error) {
	return nil, nil
}
func (m *mockQueryWatchStream) Push(ctx context.Context, tenant string, req storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error) {
	return nil, nil
}

func TestServer_StartBackgroundTasks_WatchError(t *testing.T) {
	srv := NewServer(&mockQueryWatchError{}, "", nil, Config{EnableAuth: false})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.StartBackgroundTasks(ctx)
	assert.Error(t, err)
}

func TestServer_StartBackgroundTasks_Broadcast(t *testing.T) {
	stream := make(chan storage.Event, 1)
	qs := &mockQueryWatchStream{stream: stream}
	srv := NewServer(qs, "", nil, Config{EnableAuth: false})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := srv.StartBackgroundTasks(ctx)
	assert.NoError(t, err)

	// Register a client with a subscription to capture broadcast
	client := &Client{
		hub:             srv.hub,
		queryService:    qs,
		send:            make(chan BaseMessage, 1),
		subscriptions:   map[string]Subscription{"sub": {Query: model.Query{}, IncludeData: true}},
		allowAllTenants: true,
	}
	srv.hub.Register(client)

	// Emit an event and close stream to stop watcher
	stream <- storage.Event{Id: "users/1", Type: storage.EventCreate, Document: &storage.StoredDoc{Fullpath: "users/1", Collection: "users", Data: map[string]interface{}{"foo": "bar"}}}
	close(stream)

	select {
	case msg := <-client.send:
		assert.Equal(t, TypeEvent, msg.Type)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected broadcast message")
	}
}

type mockQueryWatchBlocking struct {
	stream chan storage.Event
}

func (m *mockQueryWatchBlocking) GetDocument(ctx context.Context, tenant string, path string) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchBlocking) CreateDocument(ctx context.Context, tenant string, doc model.Document) error {
	return nil
}
func (m *mockQueryWatchBlocking) ReplaceDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchBlocking) PatchDocument(ctx context.Context, tenant string, data model.Document, pred model.Filters) (model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchBlocking) DeleteDocument(ctx context.Context, tenant string, path string, pred model.Filters) error {
	return nil
}
func (m *mockQueryWatchBlocking) ExecuteQuery(ctx context.Context, tenant string, q model.Query) ([]model.Document, error) {
	return nil, nil
}
func (m *mockQueryWatchBlocking) WatchCollection(ctx context.Context, tenant string, collection string) (<-chan storage.Event, error) {
	return m.stream, nil
}
func (m *mockQueryWatchBlocking) Pull(ctx context.Context, tenant string, req storage.ReplicationPullRequest) (*storage.ReplicationPullResponse, error) {
	return nil, nil
}
func (m *mockQueryWatchBlocking) Push(ctx context.Context, tenant string, req storage.ReplicationPushRequest) (*storage.ReplicationPushResponse, error) {
	return nil, nil
}

func TestServer_StartBackgroundTasks_ContextCancel(t *testing.T) {
	stream := make(chan storage.Event)
	qs := &mockQueryWatchBlocking{stream: stream}
	srv := NewServer(qs, "", nil, Config{EnableAuth: false})

	ctx, cancel := context.WithCancel(context.Background())

	err := srv.StartBackgroundTasks(ctx)
	assert.NoError(t, err)

	// Wait briefly for goroutine to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context - should trigger ctx.Done() branch
	cancel()

	// Goroutine should exit (we can't easily verify this, but we can check it doesn't hang)
	time.Sleep(100 * time.Millisecond)

	// Cleanup
	close(stream)
}
