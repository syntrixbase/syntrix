package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/syntrixbase/syntrix/internal/identity"
	identity_config "github.com/syntrixbase/syntrix/internal/identity/config"
	"github.com/syntrixbase/syntrix/internal/storage"
	"github.com/syntrixbase/syntrix/pkg/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockAuthStorage implements auth.StorageInterface
type MockAuthStorage struct {
	mock.Mock
}

func (m *MockAuthStorage) CreateUser(ctx context.Context, tenant string, user *storage.User) error {
	return m.Called(ctx, tenant, user).Error(0)
}
func (m *MockAuthStorage) GetUserByUsername(ctx context.Context, tenant string, username string) (*storage.User, error) {
	args := m.Called(ctx, tenant, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}
func (m *MockAuthStorage) GetUserByID(ctx context.Context, tenant string, id string) (*storage.User, error) {
	args := m.Called(ctx, tenant, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}
func (m *MockAuthStorage) UpdateUserLoginStats(ctx context.Context, tenant string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	return m.Called(ctx, tenant, id, lastLogin, attempts, lockoutUntil).Error(0)
}
func (m *MockAuthStorage) RevokeToken(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	return m.Called(ctx, tenant, jti, expiresAt).Error(0)
}
func (m *MockAuthStorage) RevokeTokenImmediate(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	return m.Called(ctx, tenant, jti, expiresAt).Error(0)
}
func (m *MockAuthStorage) IsRevoked(ctx context.Context, tenant string, jti string, gracePeriod time.Duration) (bool, error) {
	args := m.Called(ctx, tenant, jti, gracePeriod)
	return args.Bool(0), args.Error(1)
}
func (m *MockAuthStorage) ListUsers(ctx context.Context, tenant string, limit int, offset int) ([]*storage.User, error) {
	args := m.Called(ctx, tenant, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.User), args.Error(1)
}

func (m *MockAuthStorage) UpdateUser(ctx context.Context, tenant string, user *storage.User) error {
	return m.Called(ctx, tenant, user).Error(0)
}
func (m *MockAuthStorage) EnsureIndexes(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *MockAuthStorage) Close(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func TestTriggerAuth(t *testing.T) {
	// Setup Auth Service
	mockStorage := new(MockAuthStorage)
	authService, _ := identity.NewAuthN(identity_config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: time.Hour,
		AuthCodeTTL:     time.Minute,
	}, mockStorage, mockStorage)

	// Setup Server
	mockEngine := new(MockQueryService)
	server := createTestServer(mockEngine, authService, nil)

	// Generate Tokens
	// We need to access internal token generation for testing, or use SignIn/SignUp.
	// Since we are testing the handler, we can use the authService to generate system token.
	// But for user token, we need to mock SignIn or use a helper.
	// However, AuthN interface doesn't expose GenerateTokenPair directly (it's internal to SignIn/SignUp).
	// But we can use SignIn if we mock the storage correctly.
	// OR, we can just use the fact that we have the key and generate it manually using jwt-go,
	// but that duplicates logic.
	// Actually, for this test, we just need A valid token.
	// Let's use SignUp with mocked storage.

	mockStorage.On("GetUserByUsername", mock.Anything, "default", "user1").Return(nil, identity.ErrUserNotFound)
	mockStorage.On("CreateUser", mock.Anything, "default", mock.Anything).Return(nil)

	userToken, err := authService.SignUp(context.Background(), identity.SignupRequest{TenantID: "default", Username: "user1", Password: "password12345"})
	if err != nil {
		t.Fatalf("Failed to sign up: %v", err)
	}

	systemToken, _ := authService.GenerateSystemToken("trigger-worker")

	t.Run("Reject No Token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/trigger/v1/get", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Reject User Token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/trigger/v1/get", nil)
		req.Header.Set("Authorization", "Bearer "+userToken.AccessToken)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Accept System Token", func(t *testing.T) {
		// Mock Engine call because auth should pass
		mockEngine.On("GetDocument", mock.Anything, "default", "test/doc").Return(model.Document{"id": "doc1", "collection": "test", "version": int64(1)}, nil).Once()

		reqBody := `{"paths": ["test/doc"]}`
		req := httptest.NewRequest("POST", "/trigger/v1/get", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", "Bearer "+systemToken)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
