package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/pkg/benchmark/types"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		token       string
		expectError bool
	}{
		{"valid URL", "http://localhost:8080", "token123", false},
		{"valid URL with trailing slash", "http://localhost:8080/", "token123", false},
		{"empty URL", "", "token123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewHTTPClient(tt.baseURL, tt.token)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.False(t, strings.HasSuffix(client.baseURL, "/"), "baseURL should not have trailing slash")
			}
		})
	}
}

func TestHTTPClient_CreateDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/documents")
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body
		var body map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&body)
		assert.NoError(t, err)

		doc := body["doc"].(map[string]interface{})
		assert.Equal(t, "test-value", doc["test-field"])

		// Return response
		response := map[string]interface{}{
			"id":   "doc-123",
			"data": doc,
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	doc := map[string]interface{}{
		"test-field": "test-value",
	}

	result, err := client.CreateDocument(context.Background(), "test-collection", doc)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "doc-123", result.ID)
}

func TestHTTPClient_GetDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Contains(t, r.URL.Path, "/documents/doc-123")

		response := map[string]interface{}{
			"id": "doc-123",
			"data": map[string]interface{}{
				"field": "value",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	result, err := client.GetDocument(context.Background(), "test-collection", "doc-123")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "doc-123", result.ID)
}

func TestHTTPClient_UpdateDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PATCH", r.Method)
		assert.Contains(t, r.URL.Path, "/documents/doc-123")

		response := map[string]interface{}{
			"id": "doc-123",
			"data": map[string]interface{}{
				"updated": "value",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	doc := map[string]interface{}{
		"updated": "value",
	}

	result, err := client.UpdateDocument(context.Background(), "test-collection", "doc-123", doc)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "doc-123", result.ID)
}

func TestHTTPClient_DeleteDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DELETE", r.Method)
		assert.Contains(t, r.URL.Path, "/documents/doc-123")

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	err = client.DeleteDocument(context.Background(), "test-collection", "doc-123")
	assert.NoError(t, err)
}

func TestHTTPClient_Query(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/query")

		response := []map[string]interface{}{
			{
				"id": "doc-1",
				"data": map[string]interface{}{
					"name": "test1",
				},
			},
			{
				"id": "doc-2",
				"data": map[string]interface{}{
					"name": "test2",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	query := types.Query{
		Collection: "test-collection",
	}

	results, err := client.Query(context.Background(), query)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "doc-1", results[0].ID)
	assert.Equal(t, "doc-2", results[1].ID)
}

func TestHTTPClient_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "not found"}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	_, err = client.GetDocument(context.Background(), "test-collection", "nonexistent")
	assert.Error(t, err)

	httpErr, ok := GetHTTPError(err)
	assert.True(t, ok)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, httpErr.Body, "not found")
}

func TestHTTPClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "test-token")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = client.GetDocument(ctx, "test-collection", "doc-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestHTTPClient_SetToken(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", "old-token")
	require.NoError(t, err)

	assert.Equal(t, "old-token", client.token)

	client.SetToken("new-token")
	assert.Equal(t, "new-token", client.token)
}

func TestHTTPClient_GetBaseURL(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", "token")
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:8080", client.GetBaseURL())
}

func TestHTTPClient_Close(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", "token")
	require.NoError(t, err)

	err = client.Close()
	assert.NoError(t, err)
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		{"valid http", "http://localhost:8080", false, ""},
		{"valid https", "https://api.example.com", false, ""},
		{"invalid scheme", "ftp://example.com", true, "invalid URL scheme"},
		{"no scheme", "localhost:8080", false, ""}, // url.Parse doesn't fail on this
		{"invalid URL", "ht!tp://invalid", true, "invalid URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseURL(tt.url)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				if err == nil {
					assert.Equal(t, tt.url, result)
				}
			}
		})
	}
}

func TestHTTPClient_Subscribe_NotImplemented(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", "token")
	require.NoError(t, err)

	query := types.Query{Collection: "test"}
	_, err = client.Subscribe(context.Background(), query)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestHTTPClient_Unsubscribe_NotImplemented(t *testing.T) {
	client, err := NewHTTPClient("http://localhost:8080", "token")
	require.NoError(t, err)

	err = client.Unsubscribe(context.Background(), "sub-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestIsHTTPError(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 404}
	assert.True(t, IsHTTPError(httpErr))

	otherErr := assert.AnError
	assert.False(t, IsHTTPError(otherErr))
}

func TestHTTPError_Error(t *testing.T) {
	err := &HTTPError{
		StatusCode: 404,
		Status:     "Not Found",
		Body:       `{"error": "resource not found"}`,
	}

	errMsg := err.Error()
	assert.Contains(t, errMsg, "404")
	assert.Contains(t, errMsg, "Not Found")
	assert.Contains(t, errMsg, "resource not found")
}
