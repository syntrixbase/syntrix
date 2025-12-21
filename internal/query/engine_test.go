package query

import (
	"context"
	"testing"

	"syntrix/internal/common"
	"syntrix/internal/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestEngine_GetDocument(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	storedDoc := &storage.Document{Fullpath: "test/1", Collection: "test", Data: map[string]interface{}{"foo": "bar"}, Version: 2, UpdatedAt: 5, CreatedAt: 4}
	mockStorage.On("Get", ctx, "test/1").Return(storedDoc, nil)

	expectedDoc := common.Document{"foo": "bar", "id": "1", "collection": "test", "version": int64(2), "updated_at": int64(5), "created_at": int64(4)}

	doc, err := engine.GetDocument(ctx, "test/1")
	assert.NoError(t, err)
	assert.Equal(t, expectedDoc, doc)
	mockStorage.AssertExpectations(t)
}

func TestEngine_CreateDocument(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	doc := common.Document{"id": "1", "collection": "test", "foo": "bar"}
	mockStorage.On("Create", ctx, mock.MatchedBy(func(d *storage.Document) bool {
		return d.Fullpath == "test/1" && d.Collection == "test" && d.Data["foo"] == "bar"
	})).Return(nil)

	err := engine.CreateDocument(ctx, doc)
	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestEngine_ReplaceDocument_Create(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	path := "test/1"
	collection := "test"
	data := common.Document{"foo": "bar"}
	data.SetID("1")
	data.SetCollection(collection)

	// Simulate Not Found -> Create
	mockStorage.On("Get", ctx, path).Return(nil, storage.ErrNotFound)
	mockStorage.On("Create", ctx, mock.AnythingOfType("*storage.Document")).Return(nil)

	doc, err := engine.ReplaceDocument(ctx, data, storage.Filters{})
	assert.NoError(t, err)
	assert.Equal(t, "1", doc.GetID())
	assert.Equal(t, collection, doc.GetCollection())
	assert.Equal(t, "bar", doc["foo"])
	mockStorage.AssertExpectations(t)
}

func TestEngine_ReplaceDocument_Update(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	path := "test/1"
	collection := "test"
	data := common.Document{"foo": "new"}
	data.SetID("1")
	data.SetCollection(collection)
	existingDoc := &storage.Document{Id: path, Version: 1, Data: map[string]interface{}{"foo": "old"}}

	// Simulate Found -> Update -> Get
	mockStorage.On("Get", ctx, path).Return(existingDoc, nil).Once()
	mockStorage.On("Update", ctx, path, map[string]interface{}(data), storage.Filters{}).Return(nil)
	mockStorage.On("Get", ctx, path).Return(&storage.Document{Id: path, Version: 2, Data: data}, nil).Once()

	doc, err := engine.ReplaceDocument(ctx, data, storage.Filters{})
	assert.NoError(t, err)
	assert.Equal(t, "new", doc["foo"])
	assert.Equal(t, int64(2), doc["version"])
	mockStorage.AssertExpectations(t)
}

func TestEngine_PatchDocument(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	path := "test/1"
	patchData := common.Document{"bar": "baz"}
	patchData.SetID("1")
	patchData.SetCollection("test")

	patchFields := map[string]interface{}{"bar": "baz"}

	// Expect Patch call with user fields only (system fields stripped)
	mockStorage.On("Patch", ctx, path, patchFields, storage.Filters{}).Return(nil).Once()

	expectedMergedData := map[string]interface{}{"foo": "old", "bar": "baz"}
	mockStorage.On("Get", ctx, path).Return(&storage.Document{Id: path, Version: 2, Data: expectedMergedData}, nil).Once()

	doc, err := engine.PatchDocument(ctx, patchData, storage.Filters{})
	assert.NoError(t, err)
	assert.Equal(t, expectedMergedData["foo"], doc["foo"])
	assert.Equal(t, expectedMergedData["bar"], doc["bar"])
	mockStorage.AssertExpectations(t)
}

func TestEngine_ReplaceDocument_IfMatch(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	path := "test/1"
	collection := "test"
	data := common.Document{"foo": "new"}
	data.SetID("1")
	data.SetCollection(collection)
	existingDoc := &storage.Document{Id: path, Version: 1, Data: map[string]interface{}{"foo": "old"}}

	filters := storage.Filters{
		{Field: "version", Op: "==", Value: int64(1)},
	}

	// Simulate Found -> Update -> Get
	mockStorage.On("Get", ctx, path).Return(existingDoc, nil).Once()
	mockStorage.On("Update", ctx, path, map[string]interface{}(data), filters).Return(nil)
	mockStorage.On("Get", ctx, path).Return(&storage.Document{Id: path, Version: 2, Data: data}, nil).Once()

	doc, err := engine.ReplaceDocument(ctx, data, filters)
	assert.NoError(t, err)
	assert.Equal(t, "new", doc["foo"])
	assert.Equal(t, int64(2), doc["version"])
	mockStorage.AssertExpectations(t)
}

func TestEngine_PatchDocument_IfMatch(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	path := "test/1"
	patchData := common.Document{"bar": "baz"}
	patchData.SetID("1")
	patchData.SetCollection("test")

	filters := storage.Filters{
		{Field: "foo", Op: "==", Value: "old"},
	}

	// Expect Patch call with filters
	patchFields := map[string]interface{}{"bar": "baz"}
	mockStorage.On("Patch", ctx, path, patchFields, filters).Return(nil).Once()

	expectedMergedData := map[string]interface{}{"foo": "old", "bar": "baz"}
	mockStorage.On("Get", ctx, path).Return(&storage.Document{Id: path, Version: 2, Data: expectedMergedData}, nil).Once()

	doc, err := engine.PatchDocument(ctx, patchData, filters)
	assert.NoError(t, err)
	assert.Equal(t, expectedMergedData["foo"], doc["foo"])
	assert.Equal(t, expectedMergedData["bar"], doc["bar"])
	mockStorage.AssertExpectations(t)
}

func TestEngine_Pull(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	req := storage.ReplicationPullRequest{
		Collection: "test",
		Checkpoint: 100,
		Limit:      10,
	}

	expectedDocs := []*storage.Document{
		{Id: "test/1", UpdatedAt: 101},
		{Id: "test/2", UpdatedAt: 102},
	}

	mockStorage.On("Query", ctx, mock.MatchedBy(func(q storage.Query) bool {
		return q.Collection == "test" && q.Limit == 10 && q.Filters[0].Value == int64(100)
	})).Return(expectedDocs, nil)

	resp, err := engine.Pull(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, expectedDocs, resp.Documents)
	assert.Equal(t, int64(102), resp.Checkpoint)
	mockStorage.AssertExpectations(t)
}

func TestEngine_Push(t *testing.T) {
	mockStorage := new(MockStorageBackend)
	engine := NewEngine(mockStorage, "http://mock-csp")
	ctx := context.Background()

	doc := &storage.Document{Id: "test/1", Fullpath: "test/1", Collection: "test", Data: map[string]interface{}{"foo": "bar"}, Version: 1}
	req := storage.ReplicationPushRequest{
		Collection: "test",
		Changes: []storage.ReplicationPushChange{
			{Doc: doc},
		},
	}

	// Simulate Get -> Update (Success)
	existingDoc := &storage.Document{Id: "test/1", Version: 1}
	mockStorage.On("Get", ctx, "test/1").Return(existingDoc, nil)
	mockStorage.On("Update", ctx, "test/1", doc.Data, storage.Filters{}).Return(nil)

	resp, err := engine.Push(ctx, req)
	assert.NoError(t, err)
	assert.Empty(t, resp.Conflicts)
	mockStorage.AssertExpectations(t)
}
