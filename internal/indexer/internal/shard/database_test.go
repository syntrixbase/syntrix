package shard

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDatabase(t *testing.T) {
	db := NewDatabase("myapp")

	assert.Equal(t, "myapp", db.Name)
	assert.Equal(t, 0, db.ShardCount())
}

func TestShardKey(t *testing.T) {
	tests := []struct {
		pattern    string
		templateID string
		want       string
	}{
		{"users/*/chats", "ts:desc", "users/*/chats|ts:desc"},
		{"rooms/*/messages", "name:asc,age:desc", "rooms/*/messages|name:asc,age:desc"},
		{"simple", "id", "simple|id"},
	}

	for _, tt := range tests {
		got := ShardKey(tt.pattern, tt.templateID)
		assert.Equal(t, tt.want, got)
	}
}

func TestDatabase_GetShard_NotFound(t *testing.T) {
	db := NewDatabase("myapp")

	shard := db.GetShard("users/*/chats", "ts:desc")
	assert.Nil(t, shard)
}

func TestDatabase_GetOrCreateShard(t *testing.T) {
	db := NewDatabase("myapp")

	// First call creates the shard
	s1 := db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
	assert.NotNil(t, s1)
	assert.Equal(t, "users/*/chats", s1.Pattern)
	assert.Equal(t, "ts:desc", s1.TemplateID)
	assert.Equal(t, "users/{uid}/chats", s1.RawPattern)
	assert.Equal(t, 1, db.ShardCount())

	// Second call returns the same shard
	s2 := db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
	assert.Same(t, s1, s2)
	assert.Equal(t, 1, db.ShardCount())

	// Different template creates a new shard
	s3 := db.GetOrCreateShard("users/*/chats", "name:asc", "users/{uid}/chats")
	assert.NotSame(t, s1, s3)
	assert.Equal(t, 2, db.ShardCount())
}

func TestDatabase_GetShard(t *testing.T) {
	db := NewDatabase("myapp")

	// Create shard
	created := db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")

	// Get shard
	found := db.GetShard("users/*/chats", "ts:desc")
	assert.Same(t, created, found)

	// Get non-existent shard
	notFound := db.GetShard("other/*/path", "id")
	assert.Nil(t, notFound)
}

func TestDatabase_DeleteShard(t *testing.T) {
	db := NewDatabase("myapp")

	db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
	db.GetOrCreateShard("rooms/*/messages", "ts:desc", "rooms/{rid}/messages")
	assert.Equal(t, 2, db.ShardCount())

	// Delete first shard
	db.DeleteShard("users/*/chats", "ts:desc")
	assert.Equal(t, 1, db.ShardCount())
	assert.Nil(t, db.GetShard("users/*/chats", "ts:desc"))
	assert.NotNil(t, db.GetShard("rooms/*/messages", "ts:desc"))

	// Delete non-existent shard (no-op)
	db.DeleteShard("nonexistent", "id")
	assert.Equal(t, 1, db.ShardCount())
}

func TestDatabase_ListShards(t *testing.T) {
	db := NewDatabase("myapp")

	db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
	db.GetOrCreateShard("rooms/*/messages", "ts:desc", "rooms/{rid}/messages")
	db.GetOrCreateShard("users/*/chats", "name:asc", "users/{uid}/chats")

	shards := db.ListShards()
	assert.Len(t, shards, 3)

	// Verify all shards are present
	patterns := make(map[string]bool)
	for _, s := range shards {
		patterns[ShardKey(s.Pattern, s.TemplateID)] = true
	}

	assert.True(t, patterns["users/*/chats|ts:desc"])
	assert.True(t, patterns["rooms/*/messages|ts:desc"])
	assert.True(t, patterns["users/*/chats|name:asc"])
}

func TestDatabase_Clear(t *testing.T) {
	db := NewDatabase("myapp")

	db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
	db.GetOrCreateShard("rooms/*/messages", "ts:desc", "rooms/{rid}/messages")
	assert.Equal(t, 2, db.ShardCount())

	db.Clear()
	assert.Equal(t, 0, db.ShardCount())
	assert.Len(t, db.ListShards(), 0)

	// Can still create shards after clear
	db.GetOrCreateShard("new/*/path", "id", "new/{x}/path")
	assert.Equal(t, 1, db.ShardCount())
}

func TestDatabase_ConcurrentGetOrCreate(t *testing.T) {
	db := NewDatabase("myapp")

	var wg sync.WaitGroup
	shards := make([]*Shard, 100)

	// Create 100 goroutines all trying to create the same shard
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			shards[idx] = db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
		}(i)
	}

	wg.Wait()

	// All should return the same shard
	for i := 1; i < 100; i++ {
		assert.Same(t, shards[0], shards[i], "shard %d differs from shard 0", i)
	}

	// Only one shard should exist
	assert.Equal(t, 1, db.ShardCount())
}

func TestDatabase_ConcurrentAccess(t *testing.T) {
	db := NewDatabase("myapp")

	var wg sync.WaitGroup

	// Create shards concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pattern := "path" + string(rune('a'+idx%10)) + "/*/items"
			db.GetOrCreateShard(pattern, "ts:desc", pattern)
		}(i)
	}

	// Delete shards concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pattern := "path" + string(rune('a'+idx%10)) + "/*/items"
			db.DeleteShard(pattern, "ts:desc")
		}(i)
	}

	// List shards concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = db.ListShards()
		}()
	}

	wg.Wait()
}

func TestDatabase_ShardIsolation(t *testing.T) {
	db := NewDatabase("myapp")

	// Create two shards for the same pattern but different templates
	s1 := db.GetOrCreateShard("users/*/chats", "ts:desc", "users/{uid}/chats")
	s2 := db.GetOrCreateShard("users/*/chats", "name:asc", "users/{uid}/chats")

	// Insert into s1
	s1.Upsert("doc1", []byte{0x10})
	s1.Upsert("doc2", []byte{0x20})

	// Insert into s2
	s2.Upsert("doc3", []byte{0x30})

	// Verify isolation
	assert.Equal(t, 2, s1.Len())
	assert.Equal(t, 1, s2.Len())

	// Verify data
	assert.NotNil(t, s1.Get("doc1"))
	assert.NotNil(t, s1.Get("doc2"))
	assert.Nil(t, s1.Get("doc3"))

	assert.Nil(t, s2.Get("doc1"))
	assert.Nil(t, s2.Get("doc2"))
	assert.NotNil(t, s2.Get("doc3"))
}
