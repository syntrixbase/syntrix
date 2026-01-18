package types

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/syntrixbase/syntrix/internal/core/storage"
)

// ContextKey is used for storing identity data in the request context.
type ContextKey string

// Context keys shared with authentication middleware.
const (
	ContextKeyUser     ContextKey = "user"
	ContextKeyUserID   ContextKey = "user_id"
	ContextKeyUsername ContextKey = "username"
	ContextKeyRoles    ContextKey = "roles"
	ContextKeyClaims   ContextKey = "claims"
	ContextKeyDatabase ContextKey = "database"
)

// Claims represents JWT claims returned by token validation.
type Claims struct {
	Username string   `json:"username"`
	Roles    []string `json:"roles,omitempty"`
	Disabled bool     `json:"disabled"`
	Database string   `json:"database"`
	TenantID string   `json:"tid"`
	UserID   string   `json:"oid"`
	jwt.RegisteredClaims
}

// TokenPair contains access and refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// SignupRequest represents the signup payload.
type SignupRequest struct {
	Database string `json:"database,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest represents the login payload.
type LoginRequest struct {
	Database string `json:"database,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// RefreshRequest represents the refresh payload.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RuleSet defines authorization rules.
type RuleSet struct {
	Database string                `json:"database" yaml:"database"`
	Version  string                `json:"rules_version" yaml:"rules_version"`
	Service  string                `json:"service" yaml:"service"`
	Match    map[string]MatchBlock `json:"match" yaml:"match"`
}

// MatchBlock defines nested authorization rules for a path segment.
type MatchBlock struct {
	Allow map[string]string     `json:"allow" yaml:"allow"`
	Match map[string]MatchBlock `json:"match" yaml:"match"`
}

// AuthzRequest captures authorization evaluation inputs.
type AuthzRequest struct {
	Auth     Authenticated `json:"auth"`
	Resource *Resource     `json:"resource,omitempty"`
	Time     time.Time     `json:"time"`
}

// Authenticated stores authentication context for authorization evaluation.
type Authenticated struct {
	UID      interface{}            `json:"userId"`
	Username string                 `json:"username,omitempty"`
	Roles    []string               `json:"roles"`
	Claims   map[string]interface{} `json:"claims,omitempty"`
}

// Resource describes the target resource for authorization evaluation.
type Resource struct {
	Data map[string]interface{} `json:"data"`
	ID   string                 `json:"id"`
}

type User = storage.User
