package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvHelper(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Test setupServiceEnv
	env := setupServiceEnv(t, "")
	defer env.Cancel()

	// Test GenerateSystemToken
	t.Run("GenerateSystemToken", func(t *testing.T) {
		token := env.GenerateSystemToken(t)
		assert.NotEmpty(t, token)
	})

	// Test createUserInDB and GetToken
	t.Run("UserManagement", func(t *testing.T) {
		username := "testuser_helper"
		role := "user"
		
		// First creation
		env.createUserInDB(t, username, role)
		
		// Second creation (should be idempotent, covering the "exists" check)
		env.createUserInDB(t, username, role)

		token := env.GetToken(t, username, role)
		assert.NotEmpty(t, token)
	})

	// Test MakeRequest
	t.Run("MakeRequest", func(t *testing.T) {
		resp := env.MakeRequest(t, http.MethodGet, "/health", nil, "")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
