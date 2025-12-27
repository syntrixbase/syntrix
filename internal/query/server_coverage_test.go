package query

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/codetrek/syntrix/internal/storage"
	"github.com/codetrek/syntrix/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestServer_ReplaceDocument_Success(t *testing.T) {
	server, mockStorage := setupTestServer()

	docData := model.Document{"id": "1", "collection": "test", "foo": "bar"}
	pred := model.Filters{}
	path := "test/1"
	storedDoc := &storage.Document{Fullpath: path, Collection: "test", Data: map[string]interface{}{"foo": "bar"}, Version: 2}

	// ReplaceDocument logic: Get -> (if found) Update -> Get
	// Or Get -> (if not found) Create -> Get (if we test upsert create path)
	// Let's test the Update path (Replace existing)
	mockStorage.On("Get", mock.Anything, "default", path).Return(storedDoc, nil).Once()
	mockStorage.On("Update", mock.Anything, "default", path, mock.Anything, pred).Return(nil)
	mockStorage.On("Get", mock.Anything, "default", path).Return(storedDoc, nil).Once()

	reqBody, _ := json.Marshal(map[string]interface{}{
		"data":   docData,
		"pred":   pred,
		"tenant": "default",
	})
	req := httptest.NewRequest("POST", "/internal/v1/document/replace", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var respDoc model.Document
	err := json.Unmarshal(w.Body.Bytes(), &respDoc)
	assert.NoError(t, err)
	assert.Equal(t, "1", respDoc.GetID())
}

func TestServer_PatchDocument_Success(t *testing.T) {
	server, mockStorage := setupTestServer()

	docData := model.Document{"id": "1", "collection": "test", "foo": "bar"}
	pred := model.Filters{}
	path := "test/1"
	storedDoc := &storage.Document{Fullpath: path, Collection: "test", Data: map[string]interface{}{"foo": "bar"}, Version: 2}

	// PatchDocument logic: Patch -> Get
	mockStorage.On("Patch", mock.Anything, "default", path, mock.Anything, pred).Return(nil)
	mockStorage.On("Get", mock.Anything, "default", path).Return(storedDoc, nil)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"data":   docData,
		"pred":   pred,
		"tenant": "default",
	})
	req := httptest.NewRequest("POST", "/internal/v1/document/patch", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var respDoc model.Document
	err := json.Unmarshal(w.Body.Bytes(), &respDoc)
	assert.NoError(t, err)
	assert.Equal(t, "1", respDoc.GetID())
}

func TestServer_ExecuteQuery_Success(t *testing.T) {
	server, mockStorage := setupTestServer()

	q := model.Query{Collection: "test", Limit: 10}
	storedDocs := []*storage.Document{
		{Fullpath: "test/1", Collection: "test", Data: map[string]interface{}{"id": "1"}, Version: 1},
		{Fullpath: "test/2", Collection: "test", Data: map[string]interface{}{"id": "2"}, Version: 1},
	}

	mockStorage.On("Query", mock.Anything, "default", q).Return(storedDocs, nil)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"query":  q,
		"tenant": "default",
	})
	req := httptest.NewRequest("POST", "/internal/v1/query/execute", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var respDocs []model.Document
	err := json.Unmarshal(w.Body.Bytes(), &respDocs)
	assert.NoError(t, err)
	assert.Len(t, respDocs, 2)
}

func TestServer_Pull_Success(t *testing.T) {
	server, mockStorage := setupTestServer()

	pullReq := storage.ReplicationPullRequest{
		Collection: "test",
		Checkpoint: 100,
		Limit:      10,
	}
	storedDocs := []*storage.Document{
		{Fullpath: "test/1", Collection: "test", Data: map[string]interface{}{"id": "1"}, Version: 1, UpdatedAt: 101},
	}

	// Pull calls Query
	mockStorage.On("Query", mock.Anything, "default", mock.AnythingOfType("model.Query")).Return(storedDocs, nil)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"request": pullReq,
		"tenant":  "default",
	})
	req := httptest.NewRequest("POST", "/internal/replication/v1/pull", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp storage.ReplicationPullResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp.Documents, 1)
	assert.Equal(t, int64(101), resp.Checkpoint)
}

func TestServer_Push_Success(t *testing.T) {
	server, mockStorage := setupTestServer()

	doc := storage.Document{Fullpath: "test/1", Collection: "test", Data: map[string]interface{}{"id": "1"}, Version: 1}
	pushReq := storage.ReplicationPushRequest{
		Collection: "test",
		Changes: []storage.ReplicationPushChange{
			{Doc: &doc},
		},
	}

	// Push logic: Get -> (if not found) Create
	// Let's simulate a new document creation
	mockStorage.On("Get", mock.Anything, "default", "test/1").Return(nil, model.ErrNotFound)
	mockStorage.On("Create", mock.Anything, "default", mock.Anything).Return(nil)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"request": pushReq,
		"tenant":  "default",
	})
	req := httptest.NewRequest("POST", "/internal/replication/v1/push", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp storage.ReplicationPushResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Empty(t, resp.Conflicts)
}
