package query

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// MockDocumentStore is a mock implementation of storage.DocumentStore
type MockDocumentStore struct {
	mock.Mock
}

func (m *MockDocumentStore) Get(ctx context.Context, tenant, path string) (*storage.StoredDoc, error) {
	args := m.Called(ctx, tenant, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.StoredDoc), args.Error(1)
}

func (m *MockDocumentStore) Create(ctx context.Context, tenant string, doc storage.StoredDoc) error {
	args := m.Called(ctx, tenant, doc)
	return args.Error(0)
}

func (m *MockDocumentStore) Update(ctx context.Context, tenant, path string, data map[string]interface{}, pred model.Filters) error {
	args := m.Called(ctx, tenant, path, data, pred)
	return args.Error(0)
}

func (m *MockDocumentStore) Patch(ctx context.Context, tenant, path string, data map[string]interface{}, pred model.Filters) error {
	args := m.Called(ctx, tenant, path, data, pred)
	return args.Error(0)
}

func (m *MockDocumentStore) Delete(ctx context.Context, tenant, path string, pred model.Filters) error {
	args := m.Called(ctx, tenant, path, pred)
	return args.Error(0)
}

func (m *MockDocumentStore) Query(ctx context.Context, tenant string, q model.Query) ([]*storage.StoredDoc, error) {
	args := m.Called(ctx, tenant, q)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.StoredDoc), args.Error(1)
}

func (m *MockDocumentStore) Watch(ctx context.Context, tenant, collection string, resumeToken interface{}, opts storage.WatchOptions) (<-chan storage.Event, error) {
	args := m.Called(ctx, tenant, collection, resumeToken, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan storage.Event), args.Error(1)
}

func (m *MockDocumentStore) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestNewService(t *testing.T) {
	mockStore := new(MockDocumentStore)
	service := NewService(mockStore)

	assert.NotNil(t, service)
	// Verify that it implements the Service interface
	var _ Service = service
}

func TestNewClient(t *testing.T) {
	client, err := NewClient("localhost:50051")

	assert.NoError(t, err)
	assert.NotNil(t, client)
	// Verify that it implements the Service interface
	var _ Service = client
}

func TestNewHTTPHandler(t *testing.T) {
	mockStore := new(MockDocumentStore)
	service := NewService(mockStore)
	handler := NewHTTPHandler(service)

	assert.NotNil(t, handler)
	// Verify that it implements http.Handler
	var _ http.Handler = handler

	// Test that the handler responds to health check
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
}
