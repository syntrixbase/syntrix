// Package shard provides the in-memory index shard implementation.
package shard

import (
	"bytes"
	"sync"

	"github.com/google/btree"
)

// DocRef represents a document reference with its OrderKey.
type DocRef struct {
	ID       string // Document ID within the collection
	OrderKey []byte // Encoded sort key
}

// btreeItem wraps DocRef for btree storage.
type btreeItem struct {
	orderKey []byte
	id       string
}

// Shard represents an index shard for a specific (pattern, templateID) pair.
// It maintains a btree for ordered iteration and a ByID map for O(1) lookups.
type Shard struct {
	Pattern    string // Normalized pattern (e.g., "users/*/chats")
	TemplateID string // Template identity (name or fields signature)
	RawPattern string // Original pattern (e.g., "users/{uid}/chats")

	mu    sync.RWMutex
	state State // Current shard state
	tree  *btree.BTreeG[btreeItem]
	byID  map[string][]byte // docID -> OrderKey
}

// lessFunc is the comparison function for btree items.
// Items are sorted by orderKey first, then by id for deterministic ordering
// when multiple documents have the same orderKey.
func lessFunc(a, b btreeItem) bool {
	cmp := bytes.Compare(a.orderKey, b.orderKey)
	if cmp != 0 {
		return cmp < 0
	}
	return a.id < b.id
}

// NewShard creates a new index shard.
func NewShard(pattern, templateID, rawPattern string) *Shard {
	return &Shard{
		Pattern:    pattern,
		TemplateID: templateID,
		RawPattern: rawPattern,
		tree:       btree.NewG[btreeItem](32, lessFunc), // degree 32 for good performance
		byID:       make(map[string][]byte),
	}
}

// Upsert inserts or updates a document in the shard.
// If the document already exists, it removes the old entry first.
func (s *Shard) Upsert(id string, orderKey []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if present
	if oldKey, ok := s.byID[id]; ok {
		s.tree.Delete(btreeItem{orderKey: oldKey, id: id})
	}

	// Insert new entry
	s.tree.ReplaceOrInsert(btreeItem{orderKey: orderKey, id: id})
	s.byID[id] = orderKey
}

// Delete removes a document from the shard.
func (s *Shard) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldKey, ok := s.byID[id]; ok {
		s.tree.Delete(btreeItem{orderKey: oldKey, id: id})
		delete(s.byID, id)
	}
}

// SearchOptions configures a search operation.
type SearchOptions struct {
	Lower      []byte // Lower bound (inclusive)
	Upper      []byte // Upper bound (exclusive, nil means no upper bound)
	StartAfter []byte // Cursor for pagination (exclusive)
	Limit      int    // Max results to return
}

// Search returns documents within the specified bounds.
// Results are returned in OrderKey order.
func (s *Shard) Search(opts SearchOptions) []DocRef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []DocRef
	limit := opts.Limit
	if limit <= 0 {
		limit = 1000 // default limit
	}

	// Determine starting point
	startKey := opts.Lower
	isStartAfter := false
	if opts.StartAfter != nil {
		startKey = opts.StartAfter
		isStartAfter = true
	}

	// Create iterator starting point
	startItem := btreeItem{orderKey: startKey}

	s.tree.AscendGreaterOrEqual(startItem, func(item btreeItem) bool {
		// Skip the startAfter cursor itself (exclusive)
		if isStartAfter && bytes.Equal(item.orderKey, opts.StartAfter) {
			return true // continue
		}

		// Check upper bound
		if opts.Upper != nil && bytes.Compare(item.orderKey, opts.Upper) >= 0 {
			return false // stop
		}

		results = append(results, DocRef{
			ID:       item.id,
			OrderKey: item.orderKey,
		})

		return len(results) < limit
	})

	return results
}

// Get returns the OrderKey for a document by ID.
// Returns nil if the document is not indexed.
func (s *Shard) Get(id string) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byID[id]
}

// Len returns the number of documents in the shard.
func (s *Shard) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID)
}

// Clear removes all documents from the shard.
func (s *Shard) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tree.Clear(false)
	s.byID = make(map[string][]byte)
}

// State represents the current state of a shard.
type State int

const (
	StateHealthy    State = iota // Shard is up-to-date and serving queries
	StateRebuilding              // Shard is being rebuilt, queries fall back
	StateFailed                  // Shard failed, needs manual intervention
)

// String returns the state name.
func (s State) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateRebuilding:
		return "rebuilding"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// State returns the current state of the shard.
func (s *Shard) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// SetState sets the state of the shard.
func (s *Shard) SetState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

// IsHealthy returns true if the shard is in healthy state.
func (s *Shard) IsHealthy() bool {
	return s.State() == StateHealthy
}
