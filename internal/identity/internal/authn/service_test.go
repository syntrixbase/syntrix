package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/codetrek/syntrix/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) CreateUser(ctx context.Context, tenant string, user *User) error {
	args := m.Called(ctx, tenant, user)
	return args.Error(0)
}

func (m *MockStorage) GetUserByUsername(ctx context.Context, tenant string, username string) (*User, error) {
	args := m.Called(ctx, tenant, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockStorage) GetUserByID(ctx context.Context, tenant string, id string) (*User, error) {
	args := m.Called(ctx, tenant, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockStorage) UpdateUserLoginStats(ctx context.Context, tenant string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	args := m.Called(ctx, tenant, id, lastLogin, attempts, lockoutUntil)
	return args.Error(0)
}

func (m *MockStorage) RevokeToken(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	args := m.Called(ctx, tenant, jti, expiresAt)
	return args.Error(0)
}

func (m *MockStorage) RevokeTokenImmediate(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	args := m.Called(ctx, tenant, jti, expiresAt)
	return args.Error(0)
}

func (m *MockStorage) IsRevoked(ctx context.Context, tenant string, jti string, gracePeriod time.Duration) (bool, error) {
	args := m.Called(ctx, tenant, jti, gracePeriod)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) ListUsers(ctx context.Context, tenant string, limit int, offset int) ([]*User, error) {
	args := m.Called(ctx, tenant, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*User), args.Error(1)
}

func (m *MockStorage) UpdateUser(ctx context.Context, tenant string, user *User) error {
	args := m.Called(ctx, tenant, user)
	return args.Error(0)
}

func (m *MockStorage) EnsureIndexes(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestSignUp_Success(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, err := NewAuthService(cfg, mockStorage, mockStorage)
	require.NoError(t, err)
	authService := svc.(*AuthService)

	ctx := context.Background()
	req := SignupRequest{
		TenantID: "default",
		Username: "newuser",
		Password: "password12345",
	}

	// Expect GetUserByUsername to return ErrUserNotFound (checking existence)
	mockStorage.On("GetUserByUsername", ctx, "default", "newuser").Return(nil, ErrUserNotFound)

	// Expect CreateUser to be called
	mockStorage.On("CreateUser", ctx, "default", mock.Anything).Return(nil)

	tokenPair, err := authService.SignUp(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, tokenPair)
	assert.NotEmpty(t, tokenPair.AccessToken)
	assert.NotEmpty(t, tokenPair.RefreshToken)

	mockStorage.AssertExpectations(t)
}

func TestSignIn_UserNotFound(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, err := NewAuthService(cfg, mockStorage, mockStorage)
	require.NoError(t, err)

	ctx := context.Background()
	req := LoginRequest{
		TenantID: "default",
		Username: "nonexistent",
		Password: "password12345",
	}

	mockStorage.On("GetUserByUsername", ctx, "default", "nonexistent").Return(nil, ErrUserNotFound)

	tokenPair, err := authService.SignIn(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, tokenPair)
	assert.Equal(t, ErrUserNotFound, err)

	mockStorage.AssertExpectations(t)
}

func TestSignIn_Success(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.Background()
	req := LoginRequest{
		TenantID: "default",
		Username: "existinguser",
		Password: "password123",
	}

	hash, algo, _ := HashPassword("password123")
	user := &User{
		ID:           "user-id",
		TenantID:     "default",
		Username:     "existinguser",
		PasswordHash: hash,
		PasswordAlgo: algo,
	}

	mockStorage.On("GetUserByUsername", ctx, "default", "existinguser").Return(user, nil)
	mockStorage.On("UpdateUserLoginStats", ctx, "default", "user-id", mock.Anything, 0, mock.Anything).Return(nil)

	tokenPair, err := authService.SignIn(ctx, req)
	assert.NoError(t, err)
	assert.NotNil(t, tokenPair)

	mockStorage.AssertExpectations(t)
}

func TestSignIn_WrongPassword(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.Background()
	req := LoginRequest{
		TenantID: "default",
		Username: "existinguser",
		Password: "wrongpassword",
	}

	hash, algo, _ := HashPassword("password123")
	user := &User{
		ID:           "user-id",
		TenantID:     "default",
		Username:     "existinguser",
		PasswordHash: hash,
		PasswordAlgo: algo,
	}

	mockStorage.On("GetUserByUsername", ctx, "default", "existinguser").Return(user, nil)
	mockStorage.On("UpdateUserLoginStats", ctx, "default", "user-id", mock.Anything, 1, mock.Anything).Return(nil)

	tokenPair, err := authService.SignIn(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidCredentials, err)
	assert.Nil(t, tokenPair)

	mockStorage.AssertExpectations(t)
}

func TestSignIn_WrongPasswordLockout(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.Background()
	req := LoginRequest{
		TenantID: "default",
		Username: "existinguser",
		Password: "wrongpassword",
	}

	hash, algo, _ := HashPassword("password123")
	user := &User{
		ID:            "user-id",
		TenantID:      "default",
		Username:      "existinguser",
		PasswordHash:  hash,
		PasswordAlgo:  algo,
		LoginAttempts: 9,
	}

	expectedLockoutThreshold := time.Now().Add(4 * time.Minute)
	mockStorage.On("GetUserByUsername", ctx, "default", "existinguser").Return(user, nil)
	mockStorage.On("UpdateUserLoginStats", ctx, "default", "user-id", mock.Anything, 10, mock.MatchedBy(func(lockoutUntil time.Time) bool {
		return lockoutUntil.After(expectedLockoutThreshold)
	})).Return(nil)

	tokenPair, err := authService.SignIn(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidCredentials, err)
	assert.Nil(t, tokenPair)

	mockStorage.AssertExpectations(t)
}

func TestSignIn_LockedOut(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.Background()
	req := LoginRequest{TenantID: "default", Username: "locked", Password: "password123"}

	user := &User{ID: "user-id", TenantID: "default", Username: "locked", LockoutUntil: time.Now().Add(time.Hour)}
	mockStorage.On("GetUserByUsername", ctx, "default", "locked").Return(user, nil)

	tokenPair, err := authService.SignIn(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, ErrAccountLocked, err)
	assert.Nil(t, tokenPair)

	mockStorage.AssertExpectations(t)
}

func TestSignIn_Disabled(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.Background()
	req := LoginRequest{TenantID: "default", Username: "disabled", Password: "password123"}

	hash, algo, _ := HashPassword("password123")
	user := &User{ID: "user-id", TenantID: "default", Username: "disabled", PasswordHash: hash, PasswordAlgo: algo, Disabled: true}
	mockStorage.On("GetUserByUsername", ctx, "default", "disabled").Return(user, nil)

	tokenPair, err := authService.SignIn(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, ErrAccountDisabled, err)
	assert.Nil(t, tokenPair)

	mockStorage.AssertExpectations(t)
}

func TestSignUp_PasswordTooShort(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, _ := NewAuthService(cfg, mockStorage, mockStorage)
	authService := svc.(*AuthService)

	ctx := context.Background()
	req := SignupRequest{TenantID: "default", Username: "newuser", Password: "short"}

	mockStorage.On("GetUserByUsername", ctx, "default", "newuser").Return(nil, ErrUserNotFound)

	tokenPair, err := authService.SignUp(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, tokenPair)
	assert.Contains(t, err.Error(), "password too short")

	mockStorage.AssertExpectations(t)
}

func TestRefresh_Success(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, _ := NewAuthService(cfg, mockStorage, mockStorage)
	authService := svc.(*AuthService)
	tokenService := authService.tokenService

	ctx := context.Background()

	// Create a valid refresh token
	user := &User{ID: "user-id", Username: "user", TenantID: "default"}
	pair, _ := tokenService.GenerateTokenPair(user)

	mockStorage.On("IsRevoked", ctx, "default", mock.Anything, 2*time.Minute).Return(false, nil)
	mockStorage.On("GetUserByID", ctx, "default", "user-id").Return(user, nil)
	mockStorage.On("RevokeToken", ctx, "default", mock.Anything, mock.Anything).Return(nil)

	newPair, err := authService.Refresh(ctx, RefreshRequest{RefreshToken: pair.RefreshToken})
	assert.NoError(t, err)
	assert.NotNil(t, newPair)
	assert.NotEqual(t, pair.AccessToken, newPair.AccessToken)

	mockStorage.AssertExpectations(t)
}

func TestRefresh_Revoked(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, _ := NewAuthService(cfg, mockStorage, mockStorage)
	authService := svc.(*AuthService)
	tokenService := authService.tokenService

	ctx := context.Background()

	// Create a valid refresh token
	user := &User{ID: "user-id", TenantID: "default", Username: "user"}
	pair, _ := tokenService.GenerateTokenPair(user)

	mockStorage.On("IsRevoked", ctx, "default", mock.Anything, 2*time.Minute).Return(true, nil)

	newPair, err := authService.Refresh(ctx, RefreshRequest{RefreshToken: pair.RefreshToken})
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
	assert.Nil(t, newPair)

	mockStorage.AssertExpectations(t)
}

func TestRefresh_DisabledUser(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, _ := NewAuthService(cfg, mockStorage, mockStorage)
	authService := svc.(*AuthService)
	tokenService := authService.tokenService

	ctx := context.Background()

	user := &User{ID: "user-id", TenantID: "default", Username: "user", Disabled: true}
	pair, _ := tokenService.GenerateTokenPair(user)

	mockStorage.On("IsRevoked", ctx, "default", mock.Anything, 2*time.Minute).Return(false, nil)
	mockStorage.On("GetUserByID", ctx, "default", "user-id").Return(user, nil)

	newPair, err := authService.Refresh(ctx, RefreshRequest{RefreshToken: pair.RefreshToken})
	assert.Error(t, err)
	assert.Equal(t, ErrAccountDisabled, err)
	assert.Nil(t, newPair)

	mockStorage.AssertExpectations(t)
}

func TestMiddleware(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, _ := NewAuthService(cfg, mockStorage, mockStorage)
	authService := svc.(*AuthService)
	tokenService := authService.tokenService

	// Create a valid token
	user := &User{ID: "user-id", TenantID: "default", Username: "user"}
	pair, _ := tokenService.GenerateTokenPair(user)

	// Create a handler that checks context
	handler := authService.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(ContextKeyUserID)
		username := r.Context().Value(ContextKeyUsername)
		assert.Equal(t, "user-id", userID)
		assert.Equal(t, "user", username)
		w.WriteHeader(http.StatusOK)
	}))

	// Test valid token
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test missing header
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Test invalid token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLogout_InvalidToken(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.Background()

	err := authService.Logout(ctx, "not-a-token")
	assert.Equal(t, ErrInvalidToken, err)
}

func TestMiddlewareOptional(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, _ := NewAuthService(cfg, mockStorage, mockStorage)
	authService := svc.(*AuthService)
	tokenService := authService.tokenService

	user := &User{ID: "user-id", TenantID: "default", Username: "user"}
	pair, _ := tokenService.GenerateTokenPair(user)

	// No header should pass through
	called := false
	handler := authService.MiddlewareOptional(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)

	// Invalid header format
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "BadHeader")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Valid token should populate context
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthService_ListUsers(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.WithValue(context.Background(), ContextKeyTenant, "default")
	users := []*User{
		{ID: "1", Username: "user1"},
		{ID: "2", Username: "user2"},
	}

	mockStorage.On("ListUsers", ctx, "default", 10, 0).Return(users, nil)

	result, err := authService.ListUsers(ctx, 10, 0)
	assert.NoError(t, err)
	assert.Equal(t, users, result)
	mockStorage.AssertExpectations(t)
}

func TestAuthService_UpdateUser(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	authService, _ := NewAuthService(cfg, mockStorage, mockStorage)

	ctx := context.WithValue(context.Background(), ContextKeyTenant, "default")
	user := &User{ID: "1", Username: "user1", Roles: []string{"user"}, Disabled: false}

	mockStorage.On("GetUserByID", ctx, "default", "1").Return(user, nil)
	mockStorage.On("UpdateUser", ctx, "default", mock.MatchedBy(func(u *User) bool {
		return u.ID == "1" && u.Disabled == true && len(u.Roles) == 1 && u.Roles[0] == "admin"
	})).Return(nil)

	err := authService.UpdateUser(ctx, "1", []string{"admin"}, true)
	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestGenerateSystemToken(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := config.AuthNConfig{
		PrivateKeyFile:  filepath.Join(t.TempDir(), "key.pem"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		AuthCodeTTL:     2 * time.Minute,
	}
	svc, err := NewAuthService(cfg, mockStorage, mockStorage)
	require.NoError(t, err)
	authService := svc.(*AuthService)
	tokenService := authService.tokenService

	token, err := authService.GenerateSystemToken("test-service")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate the token
	claims, err := tokenService.ValidateToken(token)
	assert.NoError(t, err)
	assert.Equal(t, "system:test-service", claims.Subject)
	assert.Contains(t, claims.Roles, "system")
}
