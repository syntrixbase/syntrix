package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentSystemFields(t *testing.T) {
	env := setupServiceEnv(t, "")
	defer env.Cancel()

	token := env.GetToken(t, "test-user", "user")
	collection := "test_fields"

	// 1. Create Document
	docData := map[string]interface{}{
		"field1": "value1",
	}

	resp := env.MakeRequest(t, "POST", "/v1/"+collection, docData, token)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdDoc map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&createdDoc)
	require.NoError(t, err)
	resp.Body.Close()

	// Verify System Fields on Create
	assert.NotEmpty(t, createdDoc["id"])
	docID := createdDoc["id"].(string)

	assert.Equal(t, collection, createdDoc["collection"])
	assert.Equal(t, float64(1), createdDoc["version"]) // JSON numbers are floats
	assert.NotEmpty(t, createdDoc["created_at"])
	assert.NotEmpty(t, createdDoc["updated_at"])

	createdAt := createdDoc["created_at"].(float64)
	updatedAt := createdDoc["updated_at"].(float64)
	assert.Equal(t, createdAt, updatedAt, "On create, created_at should equal updated_at")

	// Sleep to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	// 2. Update Document (PATCH)
	patchData := map[string]interface{}{
		"doc": map[string]interface{}{
			"field2": "value2",
		},
	}
	resp = env.MakeRequest(t, "PATCH", fmt.Sprintf("/v1/%s/%s", collection, docID), patchData, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var patchedDoc map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&patchedDoc)
	require.NoError(t, err)
	resp.Body.Close()

	// Verify fields are merged (PATCH behavior)
	assert.Equal(t, "value1", patchedDoc["field1"], "Existing field should be preserved")
	assert.Equal(t, "value2", patchedDoc["field2"], "New field should be added")

	// Verify System Fields on Update
	assert.Equal(t, float64(2), patchedDoc["version"])
	assert.Equal(t, createdAt, patchedDoc["created_at"].(float64), "created_at should not change")
	assert.Greater(t, patchedDoc["updated_at"].(float64), updatedAt, "updated_at should increase")

	updatedAtAfterPatch := patchedDoc["updated_at"].(float64)

	// Sleep to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	// 3. Replace Document (PUT)
	replaceData := map[string]interface{}{
		"doc": map[string]interface{}{
			"field1": "value1-replaced",
			"field3": "value3",
		},
	}
	resp = env.MakeRequest(t, "PUT", fmt.Sprintf("/v1/%s/%s", collection, docID), replaceData, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var replacedDoc map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&replacedDoc)
	require.NoError(t, err)
	resp.Body.Close()

	// Verify System Fields on Replace
	assert.Equal(t, float64(3), replacedDoc["version"])
	assert.Equal(t, createdAt, replacedDoc["created_at"].(float64), "created_at should not change")
	assert.Greater(t, replacedDoc["updated_at"].(float64), updatedAtAfterPatch, "updated_at should increase")

	// 4. Verify Shadow Fields Protection (Try to write them)
	maliciousData := map[string]interface{}{
		"doc": map[string]interface{}{
			"field4":     "value4",
			"version":    999,
			"created_at": 0,
			"collection": "hacked",
		},
	}
	resp = env.MakeRequest(t, "PATCH", fmt.Sprintf("/v1/%s/%s", collection, docID), maliciousData, token)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var protectedDoc map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&protectedDoc)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, float64(4), protectedDoc["version"], "Server should ignore client version and increment its own")
	assert.Equal(t, createdAt, protectedDoc["created_at"].(float64), "Server should ignore client created_at")
	assert.Equal(t, collection, protectedDoc["collection"], "Server should ignore client collection")
}
