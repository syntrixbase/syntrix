package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"syntrix/internal/config"
	"syntrix/internal/services"
	"syntrix/internal/storage/mongo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentSystemFields(t *testing.T) {
	// 1. Setup Config
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	dbName := "syntrix_fields_test"

	// Clean DB
	ctx := context.Background()
	connCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	backend, err := mongo.NewMongoBackend(connCtx, mongoURI, dbName, "documents", "sys")
	if err != nil {
		t.Skipf("Skipping integration test: could not connect to MongoDB: %v", err)
	}
	// Drop database to start fresh
	backend.DB().Drop(ctx)
	backend.Close(ctx)

	apiPort := 18090 // Use different ports to avoid conflicts
	queryPort := 18091

	cfg := &config.Config{
		API: config.APIConfig{
			Port:            apiPort,
			QueryServiceURL: fmt.Sprintf("http://localhost:%d", queryPort),
		},
		Query: config.QueryConfig{
			Port:          queryPort,
			CSPServiceURL: "http://dummy-csp",
		},
		Storage: config.StorageConfig{
			MongoURI:       mongoURI,
			DatabaseName:   dbName,
			DataCollection: "documents",
			SysCollection:  "sys",
		},
	}

	opts := services.Options{
		RunAPI:   true,
		RunQuery: true,
	}

	manager := services.NewManager(cfg, opts)
	require.NoError(t, manager.Init(context.Background()))

	// Start Manager
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	manager.Start(mgrCtx)
	defer func() {
		mgrCancel()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Shutdown(shutdownCtx)
	}()

	// Wait for startup
	waitForPort(t, apiPort)
	waitForPort(t, queryPort)

	apiURL := fmt.Sprintf("http://localhost:%d", apiPort)
	client := &http.Client{Timeout: 5 * time.Second}
	collection := "test_fields"

	// 1. Create Document
	docData := map[string]interface{}{
		"field1": "value1",
	}
	body, _ := json.Marshal(docData)

	resp, err := client.Post(fmt.Sprintf("%s/v1/%s", apiURL, collection), "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createdDoc map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createdDoc)
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
	patchBody, _ := json.Marshal(patchData)
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("%s/v1/%s/%s", apiURL, collection, docID), bytes.NewBuffer(patchBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
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
	replaceBody, _ := json.Marshal(replaceData)
	req, _ = http.NewRequest("PUT", fmt.Sprintf("%s/v1/%s/%s", apiURL, collection, docID), bytes.NewBuffer(replaceBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
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
	maliciousBody, _ := json.Marshal(maliciousData)
	req, _ = http.NewRequest("PATCH", fmt.Sprintf("%s/v1/%s/%s", apiURL, collection, docID), bytes.NewBuffer(maliciousBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var protectedDoc map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&protectedDoc)
	require.NoError(t, err)
	resp.Body.Close()

	assert.Equal(t, float64(4), protectedDoc["version"], "Server should ignore client version and increment its own")
	assert.Equal(t, createdAt, protectedDoc["created_at"].(float64), "Server should ignore client created_at")
	assert.Equal(t, collection, protectedDoc["collection"], "Server should ignore client collection")
}
