package indexer

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/manager"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/mem_store"
)

// mockLocalService implements LocalService for testing.
type mockLocalService struct {
	searchFn func(ctx context.Context, database string, plan Plan) ([]DocRef, error)
	healthFn func(ctx context.Context) (Health, error)
	mgr      *manager.Manager
}

func (m *mockLocalService) Start(ctx context.Context) error { return nil }
func (m *mockLocalService) Stop(ctx context.Context) error  { return nil }
func (m *mockLocalService) ApplyEvent(ctx context.Context, evt *ChangeEvent, progress string) error {
	return nil
}
func (m *mockLocalService) Stats(ctx context.Context) (Stats, error) {
	return Stats{}, nil
}

func (m *mockLocalService) Search(ctx context.Context, database string, plan Plan) ([]DocRef, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, database, plan)
	}
	return nil, nil
}

func (m *mockLocalService) Health(ctx context.Context) (Health, error) {
	if m.healthFn != nil {
		return m.healthFn(ctx)
	}
	return Health{Status: HealthOK}, nil
}

func (m *mockLocalService) Manager() *manager.Manager {
	return m.mgr
}

func TestNewGRPCServer(t *testing.T) {
	mock := &mockLocalService{
		mgr: manager.New(mem_store.New()),
	}

	server := NewGRPCServer(mock)
	require.NotNil(t, server)
}

func TestGrpcServiceAdapter_Search(t *testing.T) {
	ctx := context.Background()

	mock := &mockLocalService{
		searchFn: func(ctx context.Context, database string, plan Plan) ([]DocRef, error) {
			assert.Equal(t, "testdb", database)
			assert.Equal(t, "users/alice/chats", plan.Collection)
			return []DocRef{
				{ID: "doc1", OrderKey: []byte{0x01, 0x02}},
			}, nil
		},
		mgr: manager.New(mem_store.New()),
	}

	docs, err := mock.Search(ctx, "testdb", Plan{
		Collection: "users/alice/chats",
	})

	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "doc1", docs[0].ID)
}

func TestGrpcServiceAdapter_Health(t *testing.T) {
	ctx := context.Background()

	mock := &mockLocalService{
		healthFn: func(ctx context.Context) (Health, error) {
			return Health{
				Status: HealthDegraded,
				Indexes: map[string]manager.IndexHealth{
					"db1|users/*/chats|ts:desc": {State: "rebuilding", DocCount: 0},
				},
			}, nil
		},
		mgr: manager.New(mem_store.New()),
	}

	health, err := mock.Health(ctx)

	require.NoError(t, err)
	assert.Equal(t, HealthDegraded, health.Status)
	assert.Equal(t, "rebuilding", health.Indexes["db1|users/*/chats|ts:desc"].State)
}

func TestGrpcServiceAdapter_HealthError(t *testing.T) {
	ctx := context.Background()

	mock := &mockLocalService{
		healthFn: func(ctx context.Context) (Health, error) {
			return Health{}, assert.AnError
		},
		mgr: manager.New(mem_store.New()),
	}

	_, err := mock.Health(ctx)
	require.Error(t, err)
}

func TestGrpcServiceAdapter_Manager(t *testing.T) {
	mgr := manager.New(mem_store.New())
	mock := &mockLocalService{mgr: mgr}

	assert.Same(t, mgr, mock.Manager())
}

func TestNewGRPCServer_Integration(t *testing.T) {
	mock := &mockLocalService{
		searchFn: func(ctx context.Context, database string, plan Plan) ([]DocRef, error) {
			return []DocRef{
				{ID: "doc1", OrderKey: []byte{0x01}},
			}, nil
		},
		healthFn: func(ctx context.Context) (Health, error) {
			return Health{Status: HealthOK}, nil
		},
		mgr: manager.New(mem_store.New()),
	}

	server := NewGRPCServer(mock)

	// The server should be ready for use - just verify it was created
	require.NotNil(t, server)
}

func TestNewClient(t *testing.T) {
	// Create a client - this exercises the NewClient function in grpc.go
	// The client.New function is already tested in client_test.go, but
	// NewClient in grpc.go wraps it
	client, err := NewClient("localhost:0", slog.Default())
	// grpc.NewClient doesn't actually connect, so this should succeed
	require.NoError(t, err)
	require.NotNil(t, client)
	client.Close()
}
