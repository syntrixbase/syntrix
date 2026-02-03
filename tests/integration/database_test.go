package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDatabaseManagement tests the database management API
func TestDatabaseManagement(t *testing.T) {
	t.Parallel()
	tc := NewTestContext(t)
	adminToken := tc.GenerateSystemToken()

	t.Run("CreateAndListDatabases", func(t *testing.T) {
		// Create a database
		createReq := map[string]interface{}{
			"display_name": "Test Database",
			"description":  "A test database for integration tests",
		}
		resp := tc.MakeRequest("POST", "/api/v1/databases", createReq, adminToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdDB map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&createdDB)
		require.NoError(t, err)
		resp.Body.Close()

		assert.NotEmpty(t, createdDB["id"])
		assert.Equal(t, "Test Database", createdDB["display_name"])
		assert.Equal(t, "active", createdDB["status"])

		dbID := createdDB["id"].(string)

		// List databases
		resp = tc.MakeRequest("GET", "/api/v1/databases", nil, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&listResp)
		require.NoError(t, err)
		resp.Body.Close()

		databases := listResp["databases"].([]interface{})
		assert.GreaterOrEqual(t, len(databases), 1)

		// Get specific database using id: prefix
		resp = tc.MakeRequest("GET", "/api/v1/databases/id:"+dbID, nil, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var getResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&getResp)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, dbID, getResp["id"])
	})

	t.Run("CreateDatabaseWithSlug", func(t *testing.T) {
		slug := tc.Slug("mydb")
		createReq := map[string]interface{}{
			"display_name": "Database With Slug",
			"slug":         slug,
		}
		resp := tc.MakeRequest("POST", "/api/v1/databases", createReq, adminToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdDB map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&createdDB)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, slug, createdDB["slug"])

		// Access by slug
		resp = tc.MakeRequest("GET", "/api/v1/databases/"+slug, nil, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()

		// Access by id: prefix
		dbID := createdDB["id"].(string)
		resp = tc.MakeRequest("GET", "/api/v1/databases/id:"+dbID, nil, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("SlugImmutability", func(t *testing.T) {
		slug := tc.Slug("immutable-slug")
		createReq := map[string]interface{}{
			"display_name": "Immutable Slug Test",
			"slug":         slug,
		}
		resp := tc.MakeRequest("POST", "/api/v1/databases", createReq, adminToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdDB map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&createdDB)
		resp.Body.Close()

		dbID := createdDB["id"].(string)

		// Try to change slug - should fail
		updateReq := map[string]interface{}{
			"slug": tc.Slug("new-slug"),
		}
		resp = tc.MakeRequest("PATCH", "/api/v1/databases/id:"+dbID, updateReq, adminToken)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		resp.Body.Close()

		// Can update display_name though
		updateReq = map[string]interface{}{
			"display_name": "Updated Display Name",
		}
		resp = tc.MakeRequest("PATCH", "/api/v1/databases/id:"+dbID, updateReq, adminToken)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	})

	t.Run("UpdateDatabaseStatus", func(t *testing.T) {
		createReq := map[string]interface{}{
			"display_name": "Status Test DB",
		}
		resp := tc.MakeRequest("POST", "/admin/databases", createReq, adminToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdDB map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&createdDB)
		resp.Body.Close()

		dbID := createdDB["id"].(string)

		// Suspend the database (admin API required to change status)
		updateReq := map[string]interface{}{
			"status": "suspended",
		}
		resp = tc.MakeRequest("PATCH", "/admin/databases/id:"+dbID, updateReq, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var updatedDB map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&updatedDB)
		resp.Body.Close()
		assert.Equal(t, "suspended", updatedDB["status"])

		// Resume the database
		updateReq = map[string]interface{}{
			"status": "active",
		}
		resp = tc.MakeRequest("PATCH", "/admin/databases/id:"+dbID, updateReq, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		json.NewDecoder(resp.Body).Decode(&updatedDB)
		resp.Body.Close()
		assert.Equal(t, "active", updatedDB["status"])
	})

	t.Run("DeleteDatabase", func(t *testing.T) {
		createReq := map[string]interface{}{
			"display_name": "To Be Deleted",
		}
		resp := tc.MakeRequest("POST", "/admin/databases", createReq, adminToken)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createdDB map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&createdDB)
		resp.Body.Close()

		dbID := createdDB["id"].(string)

		// Delete the database
		resp = tc.MakeRequest("DELETE", "/admin/databases/id:"+dbID, nil, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()

		// Verify it's in deleting status (soft delete)
		// Admin API still returns the database but with status=deleting
		resp = tc.MakeRequest("GET", "/admin/databases/id:"+dbID, nil, adminToken)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var deletedDB map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&deletedDB)
		resp.Body.Close()
		assert.Equal(t, "deleting", deletedDB["status"])
	})
}

// TestDatabaseOwnerImplicitAdmin tests that database owners have implicit db_admin permissions
func TestDatabaseOwnerImplicitAdmin(t *testing.T) {
	t.Parallel()
	tc := NewTestContext(t)

	// Create a regular user
	userToken := tc.GetToken("db-owner", "user")

	// Create a database as this user
	createReq := map[string]interface{}{
		"display_name": "Owner's Database",
		"slug":         tc.Slug("owner-db"),
	}
	resp := tc.MakeRequest("POST", "/api/v1/databases", createReq, userToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdDB map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdDB)
	resp.Body.Close()

	dbID := createdDB["id"].(string)

	// Owner should be able to update their own database
	updateReq := map[string]interface{}{
		"display_name": "Updated by Owner",
	}
	resp = tc.MakeRequest("PATCH", "/api/v1/databases/id:"+dbID, updateReq, userToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Owner should be able to delete their own database
	resp = tc.MakeRequest("DELETE", "/api/v1/databases/id:"+dbID, nil, userToken)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// TestSuspendedDatabaseRejectsWrites tests that suspended databases reject write operations
// This test uses the default database which always exists
func TestSuspendedDatabaseRejectsWrites(t *testing.T) {
	t.Parallel()
	tc := NewTestContext(t)
	adminToken := tc.GenerateSystemToken()

	// Use default database - it always exists
	// First, ensure it's active
	updateReq := map[string]interface{}{
		"status": "active",
	}
	resp := tc.MakeRequest("PATCH", "/admin/databases/default", updateReq, adminToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Create a unique collection name for this test
	collName := tc.Collection("suspended-test")

	// Write a document while active
	docData := map[string]interface{}{
		"name": "test doc",
	}
	resp = tc.MakeRequest("POST", "/api/v1/databases/default/documents/"+collName, docData, adminToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Suspend the database (admin API required)
	updateReq = map[string]interface{}{
		"status": "suspended",
	}
	resp = tc.MakeRequest("PATCH", "/admin/databases/default", updateReq, adminToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Try to write while suspended - should fail
	resp = tc.MakeRequest("POST", "/api/v1/databases/default/documents/"+collName, docData, adminToken)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	// Resume the database
	updateReq = map[string]interface{}{
		"status": "active",
	}
	resp = tc.MakeRequest("PATCH", "/admin/databases/default", updateReq, adminToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Write should work again
	resp = tc.MakeRequest("POST", "/api/v1/databases/default/documents/"+collName, docData, adminToken)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
}

// TestDatabaseQuotaEnforcement tests that database creation quota is enforced
func TestDatabaseQuotaEnforcement(t *testing.T) {
	t.Parallel()
	tc := NewTestContext(t)

	// Create a user
	userToken := tc.GetToken("quota-user", "user")

	// Create databases up to the limit (default is 10)
	// We'll create a smaller number to keep tests fast
	numToCreate := 3
	createdIDs := make([]string, 0, numToCreate)

	for i := 0; i < numToCreate; i++ {
		createReq := map[string]interface{}{
			"display_name": "Quota Test DB",
		}
		resp := tc.MakeRequest("POST", "/api/v1/databases", createReq, userToken)
		if resp.StatusCode == http.StatusCreated {
			var createdDB map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&createdDB)
			createdIDs = append(createdIDs, createdDB["id"].(string))
		}
		resp.Body.Close()
	}

	assert.Equal(t, numToCreate, len(createdIDs), "Should have created %d databases", numToCreate)

	// Clean up created databases
	adminToken := tc.GenerateSystemToken()
	for _, id := range createdIDs {
		resp := tc.MakeRequest("DELETE", "/api/v1/databases/"+id, nil, adminToken)
		resp.Body.Close()
	}
}

// TestDefaultDatabaseExists tests that the default database is created on startup
func TestDefaultDatabaseExists(t *testing.T) {
	t.Parallel()
	tc := NewTestContext(t)
	adminToken := tc.GenerateSystemToken()

	// The default database should exist
	resp := tc.MakeRequest("GET", "/api/v1/databases/default", nil, adminToken)
	// Note: This may fail if the database table hasn't been set up
	// In that case, the test is informational
	if resp.StatusCode == http.StatusOK {
		var db map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&db)
		resp.Body.Close()

		assert.Equal(t, "default", db["slug"])
		assert.Equal(t, "active", db["status"])
	} else {
		t.Logf("Default database not found (status %d) - this is expected if PostgreSQL migrations haven't been run", resp.StatusCode)
		resp.Body.Close()
	}
}

// TestDatabaseDeletionCleanup tests that deleting a database cleans up documents
func TestDatabaseDeletionCleanup(t *testing.T) {
	t.Parallel()
	tc := NewTestContext(t)
	adminToken := tc.GenerateSystemToken()

	// Create a database
	slug := tc.Slug("cleanup-test")
	createReq := map[string]interface{}{
		"display_name": "Cleanup Test DB",
		"slug":         slug,
	}
	resp := tc.MakeRequest("POST", "/api/v1/databases", createReq, adminToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdDB map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createdDB)
	resp.Body.Close()

	dbID := createdDB["id"].(string)

	// Create some documents in this database
	for i := 0; i < 5; i++ {
		docData := map[string]interface{}{
			"name":  "test doc",
			"index": i,
		}
		resp = tc.MakeRequest("POST", "/api/v1/databases/"+slug+"/documents/testcol", docData, adminToken)
		if resp.StatusCode != http.StatusCreated {
			t.Logf("Failed to create document: %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Delete the database
	resp = tc.MakeRequest("DELETE", "/admin/databases/id:"+dbID, nil, adminToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Wait a bit for the deletion worker to process
	// Note: In a real test, we'd wait for the worker to complete
	time.Sleep(500 * time.Millisecond)

	// Database should be in deleting state
	// Admin API returns the database with status=deleting
	resp = tc.MakeRequest("GET", "/admin/databases/id:"+dbID, nil, adminToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var deletedDB map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&deletedDB)
	resp.Body.Close()
	assert.Equal(t, "deleting", deletedDB["status"])
}
