package identity

import (
	"github.com/syntrixbase/syntrix/internal/core/identity/authn"
	"github.com/syntrixbase/syntrix/internal/core/identity/authz"
	"github.com/syntrixbase/syntrix/internal/core/identity/config"
	"github.com/syntrixbase/syntrix/internal/core/storage"
	"github.com/syntrixbase/syntrix/internal/query"
)

// Errors from authn
var (
	ErrInvalidCredentials = authn.ErrInvalidCredentials
	ErrAccountDisabled    = authn.ErrAccountDisabled
	ErrAccountLocked      = authn.ErrAccountLocked
	ErrInvalidToken       = authn.ErrInvalidToken
	ErrDatabaseRequired   = authn.ErrDatabaseRequired
	ErrUserNotFound       = authn.ErrUserNotFound
	ErrUserExists         = authn.ErrUserExists
)

// AuthN and AuthZ are re-exports of the internal implementations using public types.
type (
	AuthN = authn.Service
	AuthZ = authz.Engine
)

// NewAuthN creates a new authentication service.
func NewAuthN(cfg config.AuthNConfig, users storage.UserStore, revocations storage.TokenRevocationStore) (AuthN, error) {
	return authn.NewAuthService(cfg, users, revocations)
}

// NewAuthZ creates a new authorization engine.
func NewAuthZ(cfg config.AuthZConfig, qs query.Service) (AuthZ, error) {
	return authz.NewEngine(cfg, qs)
}
