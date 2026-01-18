package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/internal/core/identity"
)

func TestNewHandler_Panic(t *testing.T) {
	assert.Panics(t, func() {
		NewHandler(&MockQueryService{}, nil, nil)
	})
}

func TestGetDatabase(t *testing.T) {
	h := &Handler{}

	// Case 1: Database present
	ctx := context.WithValue(context.Background(), ContextKeyDatabase, "t1")
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	database, err := h.getDatabase(req)
	assert.NoError(t, err)
	assert.Equal(t, "t1", database)

	// Case 2: Database missing
	req2 := httptest.NewRequest("GET", "/", nil)
	database2, err2 := h.getDatabase(req2)
	assert.Error(t, err2)
	assert.Equal(t, "", database2)
}

func TestDatabaseOrError(t *testing.T) {
	h := &Handler{}

	// Case 1: Database present
	ctx := context.WithValue(context.Background(), ContextKeyDatabase, "t1")
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	database, ok := h.databaseOrError(w, req)
	assert.True(t, ok)
	assert.Equal(t, "t1", database)
	assert.Equal(t, http.StatusOK, w.Code)

	// Case 2: Database missing
	req2 := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	database2, ok2 := h.databaseOrError(w2, req2)
	assert.False(t, ok2)
	assert.Equal(t, "", database2)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestClaimsToMap(t *testing.T) {
	// Case 1: Nil claims
	assert.Nil(t, claimsToMap(nil))

	// Case 2: Valid claims
	claims := &identity.Claims{
		Database: "db1",
		TenantID: "t1",
		UserID:   "u1",
		Username: "user1",
		Roles:    []string{"admin"},
	}
	m := claimsToMap(claims)
	assert.Equal(t, "db1", m["database"])
	assert.Equal(t, "t1", m["tid"])
	assert.Equal(t, "u1", m["oid"])
	assert.Equal(t, "user1", m["username"])
	assert.Equal(t, []string{"admin"}, m["roles"])
}
