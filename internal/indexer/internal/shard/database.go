package shard

import (
	"sync"
)

// Database represents an isolated index namespace for a database.
// It contains multiple shards keyed by (pattern|templateID).
type Database struct {
	Name string

	mu     sync.RWMutex
	shards map[string]*Shard // key: pattern|templateID
}

// NewDatabase creates a new database index container.
func NewDatabase(name string) *Database {
	return &Database{
		Name:   name,
		shards: make(map[string]*Shard),
	}
}

// ShardKey generates the shard map key from pattern and templateID.
func ShardKey(pattern, templateID string) string {
	return pattern + "|" + templateID
}

// GetShard returns a shard by its key, or nil if not found.
func (d *Database) GetShard(pattern, templateID string) *Shard {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.shards[ShardKey(pattern, templateID)]
}

// GetOrCreateShard returns an existing shard or creates a new one.
func (d *Database) GetOrCreateShard(pattern, templateID, rawPattern string) *Shard {
	key := ShardKey(pattern, templateID)

	// Fast path: check with read lock
	d.mu.RLock()
	if shard, ok := d.shards[key]; ok {
		d.mu.RUnlock()
		return shard
	}
	d.mu.RUnlock()

	// Slow path: create with write lock
	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check after acquiring write lock
	if shard, ok := d.shards[key]; ok {
		return shard
	}

	shard := NewShard(pattern, templateID, rawPattern)
	d.shards[key] = shard
	return shard
}

// DeleteShard removes a shard from the database.
func (d *Database) DeleteShard(pattern, templateID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.shards, ShardKey(pattern, templateID))
}

// ListShards returns all shards in the database.
func (d *Database) ListShards() []*Shard {
	d.mu.RLock()
	defer d.mu.RUnlock()

	shards := make([]*Shard, 0, len(d.shards))
	for _, s := range d.shards {
		shards = append(shards, s)
	}
	return shards
}

// ShardCount returns the number of shards in the database.
func (d *Database) ShardCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.shards)
}

// Clear removes all shards from the database.
func (d *Database) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.shards = make(map[string]*Shard)
}
