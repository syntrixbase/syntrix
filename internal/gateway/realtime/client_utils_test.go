package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/internal/core/identity"
	api_config "github.com/syntrixbase/syntrix/internal/gateway/config"
)

func TestDatabaseFromContextMust(t *testing.T) {
	// Case 1: Database present
	ctx := context.WithValue(context.Background(), identity.ContextKeyDatabase, "t1")
	w := httptest.NewRecorder()
	database := databaseFromContextMust(ctx, w)
	assert.Equal(t, "t1", database)
	assert.Equal(t, http.StatusOK, w.Code)

	// Case 2: Database missing
	ctx2 := context.Background()
	w2 := httptest.NewRecorder()
	database2 := databaseFromContextMust(ctx2, w2)
	assert.Equal(t, "", database2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestHasSystemRoleFromClaims(t *testing.T) {
	// Case 1: Nil claims
	assert.False(t, hasSystemRoleFromClaims(nil))

	// Case 2: No system role
	claims1 := &identity.Claims{Roles: []string{"user", "admin"}}
	assert.False(t, hasSystemRoleFromClaims(claims1))

	// Case 3: System role present
	claims2 := &identity.Claims{Roles: []string{"user", "system"}}
	assert.True(t, hasSystemRoleFromClaims(claims2))

	// Case 4: System role case insensitive
	claims3 := &identity.Claims{Roles: []string{"SYSTEM"}}
	assert.True(t, hasSystemRoleFromClaims(claims3))
}

func TestTokenFromQuery(t *testing.T) {
	// Case 1: Nil request
	assert.Equal(t, "", tokenFromQuery(nil))

	// Case 2: access_token
	req1 := httptest.NewRequest("GET", "/?access_token=abc", nil)
	assert.Equal(t, "abc", tokenFromQuery(req1))

	// Case 3: token
	req2 := httptest.NewRequest("GET", "/?token=xyz", nil)
	assert.Equal(t, "xyz", tokenFromQuery(req2))

	// Case 4: Both (access_token takes precedence)
	req3 := httptest.NewRequest("GET", "/?access_token=abc&token=xyz", nil)
	assert.Equal(t, "abc", tokenFromQuery(req3))

	// Case 5: None
	req4 := httptest.NewRequest("GET", "/", nil)
	assert.Equal(t, "", tokenFromQuery(req4))
}

func TestHasCredentials(t *testing.T) {
	// Case 1: Nil request
	assert.False(t, hasCredentials(nil))

	// Case 2: No auth header
	req1 := httptest.NewRequest("GET", "/", nil)
	assert.False(t, hasCredentials(req1))

	// Case 3: Auth header present
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer token")
	assert.True(t, hasCredentials(req2))
}

func TestCheckAllowedOrigin(t *testing.T) {
	// Case 1: Empty origin, no credentials -> Allowed
	assert.NoError(t, checkAllowedOrigin("", "host", api_config.RealtimeConfig{}, false))

	// Case 2: Empty origin, credentials, dev allowed -> Allowed
	assert.NoError(t, checkAllowedOrigin("", "host", api_config.RealtimeConfig{AllowDevOrigin: true}, true))

	// Case 3: Empty origin, credentials, dev not allowed -> Error
	assert.Error(t, checkAllowedOrigin("", "host", api_config.RealtimeConfig{AllowDevOrigin: false}, true))

	// Case 4: Invalid URL -> Error
	assert.Error(t, checkAllowedOrigin(":", "host", api_config.RealtimeConfig{}, false))

	// Case 5: Same host -> Allowed
	assert.NoError(t, checkAllowedOrigin("http://host:8080", "host:8080", api_config.RealtimeConfig{}, false))

	// Case 6: Dev origin (localhost) allowed
	assert.NoError(t, checkAllowedOrigin("http://localhost:3000", "host", api_config.RealtimeConfig{AllowDevOrigin: true}, false))

	// Case 7: Dev origin (127.0.0.1) allowed
	assert.NoError(t, checkAllowedOrigin("http://127.0.0.1:3000", "host", api_config.RealtimeConfig{AllowDevOrigin: true}, false))

	// Case 8: Dev origin not allowed
	assert.Error(t, checkAllowedOrigin("http://localhost:3000", "host", api_config.RealtimeConfig{AllowDevOrigin: false}, false))

	// Case 9: Allowed origins list match
	assert.NoError(t, checkAllowedOrigin("http://example.com", "host", api_config.RealtimeConfig{AllowedOrigins: []string{"http://example.com"}}, false))

	// Case 10: Allowed origins list mismatch
	assert.Error(t, checkAllowedOrigin("http://other.com", "host", api_config.RealtimeConfig{AllowedOrigins: []string{"http://example.com"}}, false))
}

func TestDatabaseFromContext(t *testing.T) {
	// Case 1: Nil context
	database, allowAll := databaseFromContext(nil)
	assert.Equal(t, "", database)
	assert.False(t, allowAll)

	// Case 2: Database in context
	ctx1 := context.WithValue(context.Background(), identity.ContextKeyDatabase, "database1")
	database, allowAll = databaseFromContext(ctx1)
	assert.Equal(t, "database1", database)
	assert.False(t, allowAll)

	// Case 3: Database in claims
	claims := &identity.Claims{Database: "database2"}
	ctx2 := context.WithValue(context.Background(), identity.ContextKeyClaims, claims)
	database, allowAll = databaseFromContext(ctx2)
	assert.Equal(t, "database2", database)
	assert.False(t, allowAll)

	// Case 4: Database in both (ContextKeyDatabase takes precedence)
	ctx3 := context.WithValue(ctx2, identity.ContextKeyDatabase, "database1")
	database, allowAll = databaseFromContext(ctx3)
	assert.Equal(t, "database1", database)
	assert.False(t, allowAll)

	// Case 5: System role
	claimsSystem := &identity.Claims{Database: "database3", Roles: []string{"system"}}
	ctx4 := context.WithValue(context.Background(), identity.ContextKeyClaims, claimsSystem)
	database, allowAll = databaseFromContext(ctx4)
	assert.Equal(t, "database3", database)
	assert.True(t, allowAll)
}
