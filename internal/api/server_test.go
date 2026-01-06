package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/api/realtime"
	"github.com/syntrixbase/syntrix/internal/engine"
	"github.com/syntrixbase/syntrix/internal/identity"
	"github.com/syntrixbase/syntrix/internal/identity/types"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// Mocks
type MockQueryService struct {
	mock.Mock
	engine.Service
}

func (m *MockQueryService) GetDocument(ctx context.Context, tenant string, path string) (model.Document, error) {
	args := m.Called(ctx, tenant, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(model.Document), args.Error(1)
}

type MockAuthService struct {
	mock.Mock
	identity.AuthN
}

func (m *MockAuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), types.ContextKeyTenant, "default")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *MockAuthService) MiddlewareOptional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), types.ContextKeyTenant, "default")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type MockAuthzEngine struct {
	mock.Mock
	identity.AuthZ
}

func (m *MockAuthzEngine) Evaluate(ctx context.Context, path string, action string, req identity.AuthzRequest, existingRes *identity.Resource) (bool, error) {
	args := m.Called(ctx, path, action, req, existingRes)
	return args.Bool(0), args.Error(1)
}

func TestNewServer(t *testing.T) {
	mockQuery := new(MockQueryService)
	mockAuth := new(MockAuthService)
	mockAuthz := new(MockAuthzEngine)

	server := NewServer(mockQuery, mockAuth, mockAuthz, nil)
	assert.NotNil(t, server)
	assert.NotNil(t, server.rest)
}

func TestNewServer_WithRealtime(t *testing.T) {
	mockQuery := new(MockQueryService)
	mockAuth := new(MockAuthService)
	mockAuthz := new(MockAuthzEngine)

	rt := realtime.NewServer(mockQuery, "docs", mockAuth, realtime.Config{EnableAuth: true})

	server := NewServer(mockQuery, mockAuth, mockAuthz, rt)
	assert.NotNil(t, server)
	assert.NotNil(t, server.realtime)
}

func TestServer_RegisterRoutes(t *testing.T) {
	mockQuery := new(MockQueryService)
	mockAuth := new(MockAuthService)
	mockAuthz := new(MockAuthzEngine)

	server := NewServer(mockQuery, mockAuth, mockAuthz, nil)

	// Create a mux and register routes
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	t.Run("Routes registered - health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// Health endpoint should respond (may be 200 or error depending on engine state)
		// The key is that the route is registered and handled
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("Routes registered - API endpoint", func(t *testing.T) {
		// Mock GetDocument to return ErrNotFound
		// Use valid document path format: collection/docId (even number of segments)
		mockQuery.On("GetDocument", mock.Anything, "default", "users/test").Return(nil, model.ErrNotFound)
		mockAuthz.On("Evaluate", mock.Anything, "users/test", "read", mock.Anything, mock.Anything).Return(true, nil)

		req := httptest.NewRequest("GET", "/api/v1/users/test", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// Route should be registered (404 from handler, not from mux)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestServer_RegisterRoutes_WithRealtime(t *testing.T) {
	mockQuery := new(MockQueryService)
	mockAuth := new(MockAuthService)
	mockAuthz := new(MockAuthzEngine)

	rt := realtime.NewServer(mockQuery, "docs", mockAuth, realtime.Config{EnableAuth: true})
	server := NewServer(mockQuery, mockAuth, mockAuthz, rt)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	t.Run("Realtime WS route registered", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/realtime/ws", nil)
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// Should not be 404 (route exists, may fail for other reasons like missing upgrade header)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("Realtime SSE route registered", func(t *testing.T) {
		// For SSE, we can't easily test the full flow because it requires Hub.Run().
		// Instead, we verify the route exists by checking that the response is not 404.
		// We use a mock approach: call with invalid method to confirm route exists.
		req := httptest.NewRequest("POST", "/realtime/sse", nil) // SSE only accepts GET
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// 405 Method Not Allowed means route exists but method is wrong
		// This confirms the route is registered without triggering the blocking SSE handler
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
