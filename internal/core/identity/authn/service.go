package authn

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/syntrixbase/syntrix/internal/core/identity/config"
	"github.com/syntrixbase/syntrix/internal/core/storage"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrAccountLocked      = errors.New("account locked")
	ErrInvalidToken       = errors.New("invalid token")
	ErrDatabaseRequired   = errors.New("database is required")
)

type UserStore = storage.UserStore
type TokenRevocationStore = storage.TokenRevocationStore

type Service interface {
	Middleware(next http.Handler) http.Handler
	MiddlewareOptional(next http.Handler) http.Handler
	SignIn(ctx context.Context, req LoginRequest) (*TokenPair, error)
	SignUp(ctx context.Context, req SignupRequest) (*TokenPair, error)
	Refresh(ctx context.Context, req RefreshRequest) (*TokenPair, error)
	ListUsers(ctx context.Context, limit int, offset int) ([]*User, error)
	UpdateUser(ctx context.Context, id string, roles []string, disabled bool) error
	Logout(ctx context.Context, refreshToken string) error
	GenerateSystemToken(serviceName string) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
}

type AuthService struct {
	users        UserStore
	revocations  TokenRevocationStore
	tokenService *TokenService
}

func NewAuthService(cfg config.AuthNConfig, users UserStore, revocations TokenRevocationStore) (Service, error) {
	tokenService, err := NewTokenService(cfg)
	if err != nil {
		return nil, err
	}
	return &AuthService{
		users:        users,
		revocations:  revocations,
		tokenService: tokenService,
	}, nil
}

func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	return s.tokenService.ValidateToken(tokenString)
}

func (s *AuthService) SignIn(ctx context.Context, req LoginRequest) (*TokenPair, error) {
	database := req.Database
	if database == "" {
		return nil, ErrDatabaseRequired
	}

	user, err := s.users.GetUserByUsername(ctx, database, req.Username)
	if err != nil {
		return nil, err
	}

	// Check lockout
	if user.LockoutUntil.After(time.Now()) {
		return nil, ErrAccountLocked
	}

	// Verify password
	valid, err := VerifyPassword(req.Password, user.PasswordHash, user.PasswordAlgo)
	if err != nil {
		return nil, err
	}

	if !valid {
		// Handle failed attempt
		attempts := user.LoginAttempts + 1
		lockoutUntil := user.LockoutUntil
		if attempts >= 10 { // Lockout threshold
			lockoutUntil = time.Now().Add(5 * time.Minute)
		}
		_ = s.users.UpdateUserLoginStats(ctx, database, user.ID, user.LastLoginAt, attempts, lockoutUntil)
		return nil, ErrInvalidCredentials
	}

	// Check disabled
	if user.Disabled {
		return nil, ErrAccountDisabled
	}

	// Success - reset stats
	_ = s.users.UpdateUserLoginStats(ctx, database, user.ID, time.Now(), 0, time.Time{})

	return s.tokenService.GenerateTokenPair(user)
}

func (s *AuthService) SignUp(ctx context.Context, req SignupRequest) (*TokenPair, error) {
	database := req.Database
	if database == "" {
		return nil, ErrDatabaseRequired
	}

	// Check if user already exists
	_, err := s.users.GetUserByUsername(ctx, database, req.Username)
	if err == nil {
		return nil, errors.New("user already exists")
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	// Validate password strength
	if len(req.Password) < 12 {
		return nil, errors.New("password too short (min 12 chars)")
	}
	// TODO: Add complexity check and breached password check

	hash, algo, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Database:     database,
		PasswordHash: hash,
		PasswordAlgo: algo,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Disabled:     false,
		Roles:        []string{}, // Default roles
	}

	// Assign admin role to default superuser
	if user.Username == "syntrix" {
		user.Roles = append(user.Roles, "admin")
	} else {
		user.Roles = append(user.Roles, "user")
	}

	if err := s.users.CreateUser(ctx, database, user); err != nil {
		return nil, err
	}

	return s.tokenService.GenerateTokenPair(user)
}

func (s *AuthService) Refresh(ctx context.Context, req RefreshRequest) (*TokenPair, error) {
	// TODO: Extract database from request or context
	claims, err := s.tokenService.ValidateToken(req.RefreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Check revocation with overlap
	revoked, err := s.revocations.IsRevoked(ctx, claims.Database, claims.ID, s.tokenService.RefreshOverlap())
	if err != nil {
		return nil, err
	}
	if revoked {
		return nil, ErrInvalidToken // Revoked
	}

	// Get user to ensure still exists/active
	user, err := s.users.GetUserByID(ctx, claims.Database, claims.Subject)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if user.Disabled {
		return nil, ErrAccountDisabled
	}

	// Revoke old token (soft revoke for overlap)
	// We use the Expiration time from claims to clean up later
	if err := s.revocations.RevokeToken(ctx, claims.Database, claims.ID, claims.ExpiresAt.Time); err != nil {
		return nil, err
	}

	// Issue new pair
	return s.tokenService.GenerateTokenPair(user)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokenService.ValidateToken(refreshToken)
	if err != nil {
		return ErrInvalidToken
	}

	return s.revocations.RevokeTokenImmediate(ctx, claims.Database, claims.ID, claims.ExpiresAt.Time)
}

func (s *AuthService) GenerateSystemToken(serviceName string) (string, error) {
	return s.tokenService.GenerateSystemToken(serviceName)
}

func (s *AuthService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims, err := s.tokenService.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		if claims.Database == "" {
			http.Error(w, "Invalid token: missing database ID", http.StatusUnauthorized)
			return
		}

		// Add user info to context
		ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.Subject)
		ctx = context.WithValue(ctx, ContextKeyUsername, claims.Username)
		ctx = context.WithValue(ctx, ContextKeyRoles, claims.Roles)
		ctx = context.WithValue(ctx, ContextKeyClaims, claims)
		ctx = context.WithValue(ctx, ContextKeyDatabase, claims.Database)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *AuthService) MiddlewareOptional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims, err := s.tokenService.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		if claims.Database == "" {
			http.Error(w, "Invalid token: missing database ID", http.StatusUnauthorized)
			return
		}

		// Add user info to context
		ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.Subject)
		ctx = context.WithValue(ctx, ContextKeyUsername, claims.Username)
		ctx = context.WithValue(ctx, ContextKeyRoles, claims.Roles)
		ctx = context.WithValue(ctx, ContextKeyClaims, claims)
		ctx = context.WithValue(ctx, ContextKeyDatabase, claims.Database)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *AuthService) ListUsers(ctx context.Context, limit int, offset int) ([]*User, error) {
	database, err := databaseFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.users.ListUsers(ctx, database, limit, offset)
}

func (s *AuthService) UpdateUser(ctx context.Context, id string, roles []string, disabled bool) error {
	database, err := databaseFromContext(ctx)
	if err != nil {
		return err
	}

	user, err := s.users.GetUserByID(ctx, database, id)
	if err != nil {
		return err
	}

	user.Roles = roles
	user.Disabled = disabled
	return s.users.UpdateUser(ctx, database, user)
}

func databaseFromContext(ctx context.Context) (string, error) {
	val := ctx.Value(ContextKeyDatabase)
	if val == nil {
		return "", ErrDatabaseRequired
	}

	database, ok := val.(string)
	if !ok || database == "" {
		return "", ErrDatabaseRequired
	}

	return database, nil
}
