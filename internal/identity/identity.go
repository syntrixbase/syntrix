package identity

import (
	"context"
	"net/http"

	"github.com/codetrek/syntrix/internal/config"
	"github.com/codetrek/syntrix/internal/identity/internal/authn"
	"github.com/codetrek/syntrix/internal/identity/internal/authz"
	"github.com/codetrek/syntrix/internal/query"
	"github.com/codetrek/syntrix/internal/storage"
)

// Type aliases for authn types
type (
	ContextKey     = authn.ContextKey
	LoginRequest   = authn.LoginRequest
	RefreshRequest = authn.RefreshRequest
	TokenPair      = authn.TokenPair
	User           = authn.User
	Claims         = authn.Claims
)

// Constants for context keys
const (
	ContextKeyUser     = authn.ContextKeyUser
	ContextKeyUserID   = authn.ContextKeyUserID
	ContextKeyUsername = authn.ContextKeyUsername
	ContextKeyRoles    = authn.ContextKeyRoles
)

// Errors from authn
var (
	ErrInvalidCredentials = authn.ErrInvalidCredentials
	ErrAccountDisabled    = authn.ErrAccountDisabled
	ErrAccountLocked      = authn.ErrAccountLocked
	ErrInvalidToken       = authn.ErrInvalidToken
	ErrUserNotFound       = authn.ErrUserNotFound
	ErrUserExists         = authn.ErrUserExists
)

// Type aliases for authz types
type (
	Request  = authz.Request
	Resource = authz.Resource
	RuleSet  = authz.RuleSet
)

// AuthN defines the authentication interface.
type AuthN interface {
	Middleware(next http.Handler) http.Handler
	MiddlewareOptional(next http.Handler) http.Handler
	SignIn(ctx context.Context, req LoginRequest) (*TokenPair, error)
	SignUp(ctx context.Context, req LoginRequest) (*TokenPair, error)
	Refresh(ctx context.Context, req RefreshRequest) (*TokenPair, error)
	ListUsers(ctx context.Context, limit int, offset int) ([]*User, error)
	UpdateUser(ctx context.Context, id string, roles []string, disabled bool) error
	Logout(ctx context.Context, refreshToken string) error
	GenerateSystemToken(serviceName string) (string, error)
	ValidateToken(tokenString string) (*Claims, error)
}

// AuthZ defines the authorization interface.
type AuthZ interface {
	Evaluate(ctx context.Context, path string, action string, req Request, existingRes *Resource) (bool, error)
	GetRules() *RuleSet
	UpdateRules(content []byte) error
	LoadRules(path string) error
}

// NewAuthN creates a new authentication service.
func NewAuthN(cfg config.AuthNConfig, users storage.UserStore, revocations storage.TokenRevocationStore) (AuthN, error) {
	return authn.NewAuthService(cfg, users, revocations)
}

// NewAuthZ creates a new authorization engine.
func NewAuthZ(cfg config.AuthZConfig, qs query.Service) (AuthZ, error) {
	return authz.NewEngine(cfg, qs)
}
