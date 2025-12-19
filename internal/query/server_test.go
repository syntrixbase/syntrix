package query

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"syntrix/internal/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupTestServer() (*Server, *MockStorageBackend) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	server := NewServer(engine)
	return server, mockStorage
}

func TestServer_GetDocument(t *testing.T) {
	server, mockStorage := setupTestServer()

	path := "test/1"
	doc := &storage.Document{Id: path, Data: map[string]interface{}{"foo": "bar"}}
	mockStorage.On("Get", mock.Anything, path).Return(doc, nil)

	reqBody, _ := json.Marshal(map[string]string{"path": path})
	req := httptest.NewRequest("POST", "/internal/v1/document/get", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var respDoc storage.Document
	err := json.Unmarshal(w.Body.Bytes(), &respDoc)
	assert.NoError(t, err)
	assert.Equal(t, doc.Id, respDoc.Id)
}

func TestServer_CreateDocument(t *testing.T) {
	server, mockStorage := setupTestServer()

	doc := &storage.Document{Id: "test/1", Data: map[string]interface{}{"foo": "bar"}}
	mockStorage.On("Create", mock.Anything, mock.AnythingOfType("*storage.Document")).Return(nil)

	reqBody, _ := json.Marshal(doc)
	req := httptest.NewRequest("POST", "/internal/v1/document/create", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestServer_DeleteDocument(t *testing.T) {
	server, mockStorage := setupTestServer()

	path := "test/1"
	mockStorage.On("Delete", mock.Anything, path).Return(nil)

	reqBody, _ := json.Marshal(map[string]string{"path": path})
	req := httptest.NewRequest("POST", "/internal/v1/document/delete", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
