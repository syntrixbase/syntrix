package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateDatabaseID(t *testing.T) {
	id1 := CalculateDatabaseID("database1", "/path/to/doc1")
	id2 := CalculateDatabaseID("database1", "/path/to/doc1")
	id3 := CalculateDatabaseID("database2", "/path/to/doc1")
	id4 := CalculateDatabaseID("database1", "/path/to/doc2")

	assert.Equal(t, id1, id2)
	assert.NotEqual(t, id1, id3)
	assert.NotEqual(t, id1, id4)
	assert.True(t, strings.HasPrefix(id1, "database1:"))
}

func TestCalculateID(t *testing.T) {
	id1 := CalculateID("/path/to/doc1")
	id2 := CalculateID("/path/to/doc1")
	id3 := CalculateID("/path/to/doc2")

	assert.Equal(t, id1, id2, "Same path should generate same ID")
	assert.NotEqual(t, id1, id3, "Different paths should generate different IDs")
	assert.NotEmpty(t, id1)
}

func TestCalculateCollectionHash(t *testing.T) {
	h1 := CalculateCollectionHash("users")
	h2 := CalculateCollectionHash("users")
	h3 := CalculateCollectionHash("orders")

	assert.Equal(t, h1, h2)
	assert.NotEqual(t, h1, h3)
	assert.NotEmpty(t, h1)
}

func TestNewDocument(t *testing.T) {
	data := map[string]interface{}{
		"key": "value",
	}
	doc := NewStoredDoc("database1", "users", "123", data)

	assert.Equal(t, "database1", doc.DatabaseID)
	assert.Equal(t, "users/123", doc.Fullpath)
	assert.Equal(t, "users", doc.Collection)
	assert.Equal(t, data, doc.Data)
	assert.NotEmpty(t, doc.Id)
	assert.Equal(t, CalculateDatabaseID("database1", "users/123"), doc.Id)
	assert.Equal(t, CalculateCollectionHash("users"), doc.CollectionHash)
	assert.NotZero(t, doc.CreatedAt)
	assert.NotZero(t, doc.UpdatedAt)
}

func TestNewDocument_NoSlashCollection(t *testing.T) {
	data := map[string]interface{}{"key": "value"}
	doc := NewStoredDoc("database1", "root", "", data)

	assert.Equal(t, "root", doc.Collection)
	assert.Empty(t, doc.Parent)
}
