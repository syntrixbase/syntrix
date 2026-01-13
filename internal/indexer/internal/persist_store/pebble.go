package persist_store

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/store"
)

// Config configures the PebbleStore.
type Config struct {
	// Path is the directory to store the database.
	Path string `yaml:"path"`

	// BatchSize is the max number of operations per batch.
	BatchSize int `yaml:"batch_size"`

	// BatchInterval is the max time to wait before flushing a batch.
	BatchInterval time.Duration `yaml:"batch_interval"`

	// QueueSize is the buffer for pending writes.
	QueueSize int `yaml:"queue_size"`

	// BlockCacheSize is the size of the block cache in bytes.
	BlockCacheSize int64 `yaml:"block_cache_size"`

	// Logger for store operations.
	Logger *slog.Logger `yaml:"-"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Path:           "data/indexer/indexes.db",
		BatchSize:      100,
		BatchInterval:  100 * time.Millisecond,
		QueueSize:      10000,
		BlockCacheSize: 128 * 1024 * 1024, // 128MB
	}
}

// PebbleStore implements Store using PebbleDB.
type PebbleStore struct {
	db     *pebble.DB
	path   string
	logger *slog.Logger

	// Async batching
	mu                  sync.RWMutex
	pending             map[string]*pendingOp // key: "{db}|{hash}|{docID}"
	flushing            map[string]*pendingOp
	pendingProgress     string                   // progress to save with next batch
	pendingIndexDeletes map[string]indexDeleteOp // key: "{db}|{hash}"
	notifyCh            chan struct{}
	closeCh             chan struct{}
	closed              bool
	flushDoneCh         chan struct{} // buffered(1), signaled after each flush

	batchSize     int
	batchInterval time.Duration
	batcherWG     sync.WaitGroup
}

// indexDeleteOp represents a pending index delete operation.
type indexDeleteOp struct {
	db      string
	pattern string
	tmplID  string
	hash    string
}

// pendingOp represents a pending write operation.
type pendingOp struct {
	db       string
	pattern  string
	tmplID   string
	hash     string // hex(xxHash64(pattern + "|" + tmplID))
	docID    string
	orderKey []byte // nil = delete

	// Precomputed keys for batch apply
	idxKey []byte // idx/{db}/{hash}/{orderKey}
	revKey []byte // rev/{db}/{hash}/{docID}
	mapKey []byte // map/{db}/{hash}
}

// NewPebbleStore creates a new PebbleStore.
func NewPebbleStore(cfg Config) (*PebbleStore, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("store path is required")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "index-store")

	// Ensure directory exists
	if err := os.MkdirAll(cfg.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	// Configure PebbleDB
	cache := pebble.NewCache(cfg.BlockCacheSize)
	defer cache.Unref()

	dbOpts := &pebble.Options{
		Cache: cache,
		Levels: []pebble.LevelOptions{
			{FilterPolicy: bloom.FilterPolicy(10)}, // 10 bits per key, ~1% false positive
		},
	}

	db, err := pebble.Open(cfg.Path, dbOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open pebble database: %w", err)
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}
	batchInterval := cfg.BatchInterval
	if batchInterval <= 0 {
		batchInterval = 100 * time.Millisecond
	}

	s := &PebbleStore{
		db:                  db,
		path:                cfg.Path,
		logger:              logger,
		pending:             make(map[string]*pendingOp),
		flushing:            make(map[string]*pendingOp),
		pendingIndexDeletes: make(map[string]indexDeleteOp),
		notifyCh:            make(chan struct{}, 1),
		closeCh:             make(chan struct{}),
		flushDoneCh:         make(chan struct{}, 1),
		batchSize:           batchSize,
		batchInterval:       batchInterval,
	}

	s.startBatcher()

	return s, nil
}

// Close closes the store.
func (s *PebbleStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	// Signal batcher to stop
	close(s.closeCh)

	// Wait for batcher to finish
	s.batcherWG.Wait()

	// Close PebbleDB
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("failed to close pebble database: %w", err)
	}

	return nil
}

// Flush forces all pending writes to disk and waits for completion.
func (s *PebbleStore) Flush() error {
	// Timeout: max of 5s or 50x batchInterval
	timeout := 5 * time.Second
	if t := s.batchInterval * 50; t > timeout {
		timeout = t
	}
	deadline := time.Now().Add(timeout)

	for {
		// Check if there's anything pending
		s.mu.RLock()
		hasPending := len(s.pending) > 0 || len(s.flushing) > 0 || s.pendingProgress != "" || len(s.pendingIndexDeletes) > 0
		s.mu.RUnlock()

		if !hasPending {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("flush timeout")
		}

		// Trigger flush
		select {
		case s.notifyCh <- struct{}{}:
		default:
		}

		// Wait for batcher to signal completion or timeout
		select {
		case <-s.flushDoneCh:
			// Flush completed, check again
		case <-time.After(100 * time.Millisecond):
			// Fallback timeout in case signal was missed
		}
	}
}

// startBatcher starts the background batcher goroutine.
func (s *PebbleStore) startBatcher() {
	s.batcherWG.Add(1)
	go func() {
		defer s.batcherWG.Done()
		s.batcherLoop()
	}()
}

// batcherLoop is the main loop for the batcher goroutine.
func (s *PebbleStore) batcherLoop() {
	ticker := time.NewTicker(s.batchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeCh:
			// Final flush before exit
			s.doFlush()
			return
		case <-s.notifyCh:
			s.maybeFlush()
		case <-ticker.C:
			s.maybeFlush()
		}
	}
}

// maybeFlush flushes if there are enough pending ops.
// Called from ticker (timeout-based) and notifyCh (triggered).
func (s *PebbleStore) maybeFlush() {
	s.mu.RLock()
	pendingCount := len(s.pending)
	s.mu.RUnlock()

	if pendingCount == 0 {
		return
	}

	// Always flush on maybeFlush - either batchSize reached or timeout
	s.doFlush()
}

// doFlush performs the actual flush operation.
func (s *PebbleStore) doFlush() {
	// Swap pending to flushing and capture progress and index deletes
	s.mu.Lock()
	if len(s.pending) == 0 && s.pendingProgress == "" && len(s.pendingIndexDeletes) == 0 {
		s.mu.Unlock()
		return
	}
	s.flushing = s.pending
	s.pending = make(map[string]*pendingOp)
	progressToSave := s.pendingProgress
	s.pendingProgress = ""
	indexDeletes := s.pendingIndexDeletes
	s.pendingIndexDeletes = make(map[string]indexDeleteOp)
	s.mu.Unlock()

	// Build and commit batch for document operations
	if len(s.flushing) > 0 || progressToSave != "" {
		batch := s.db.NewBatch()
		for _, op := range s.flushing {
			if err := s.applyOp(batch, op); err != nil {
				s.logger.Error("failed to apply operation", "error", err)
			}
		}

		// Save progress atomically with index data
		if progressToSave != "" {
			if err := batch.Set([]byte(keyProgress), []byte(progressToSave), nil); err != nil {
				s.logger.Error("failed to set progress in batch", "error", err)
			}
		}

		if err := batch.Commit(pebble.Sync); err != nil {
			s.logger.Error("failed to commit batch", "error", err)
		}
		batch.Close()
	}

	// Clear flushing and signal completion
	s.mu.Lock()
	s.flushing = make(map[string]*pendingOp)
	s.mu.Unlock()

	// Signal flush completion (non-blocking)
	select {
	case s.flushDoneCh <- struct{}{}:
	default:
	}

	// Process index deletes (outside the batch since they use deleteByPrefix)
	for _, delOp := range indexDeletes {
		if err := s.executeIndexDelete(delOp); err != nil {
			s.logger.Error("failed to delete index", "db", delOp.db, "pattern", delOp.pattern, "error", err)
		}
	}
}

// executeIndexDelete performs the actual index deletion.
func (s *PebbleStore) executeIndexDelete(delOp indexDeleteOp) error {
	// Delete all idx entries
	idxPrefix := indexKeyPrefix(delOp.db, delOp.pattern, delOp.tmplID)
	if err := s.deleteByPrefix(idxPrefix); err != nil {
		return fmt.Errorf("failed to delete index entries: %w", err)
	}

	// Delete all rev entries
	revPrefix := reverseKeyPrefix(delOp.db, delOp.pattern, delOp.tmplID)
	if err := s.deleteByPrefix(revPrefix); err != nil {
		return fmt.Errorf("failed to delete reverse entries: %w", err)
	}

	// Delete map entry
	mKey := mapKey(delOp.db, delOp.pattern, delOp.tmplID)
	if err := s.db.Delete(mKey, pebble.Sync); err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("failed to delete map entry: %w", err)
	}

	// Delete state entry
	sKey := stateKey(delOp.db, delOp.pattern, delOp.tmplID)
	if err := s.db.Delete(sKey, pebble.Sync); err != nil && err != pebble.ErrNotFound {
		return fmt.Errorf("failed to delete state entry: %w", err)
	}

	return nil
}

// applyOp applies a single pending operation to the batch.
func (s *PebbleStore) applyOp(batch *pebble.Batch, op *pendingOp) error {
	if op.orderKey == nil {
		// Delete operation
		// Need to look up existing orderKey to delete idx entry
		oldOrderKey, closer, err := s.db.Get(op.revKey)
		if err == pebble.ErrNotFound {
			return nil // Already deleted, idempotent
		}
		if err != nil {
			return fmt.Errorf("failed to read reverse index: %w", err)
		}
		closer.Close()

		// Delete index entry
		oldIdxKey := indexKey(op.db, op.pattern, op.tmplID, oldOrderKey)
		if err := batch.Delete(oldIdxKey, nil); err != nil {
			return fmt.Errorf("failed to delete index entry: %w", err)
		}

		// Delete reverse index
		if err := batch.Delete(op.revKey, nil); err != nil {
			return fmt.Errorf("failed to delete reverse index: %w", err)
		}
	} else {
		// Upsert operation
		// First delete old index entry if exists
		oldOrderKey, closer, err := s.db.Get(op.revKey)
		if err != nil && err != pebble.ErrNotFound {
			return fmt.Errorf("failed to read reverse index: %w", err)
		}
		if closer != nil {
			if oldOrderKey != nil {
				oldIdxKey := indexKey(op.db, op.pattern, op.tmplID, oldOrderKey)
				if err := batch.Delete(oldIdxKey, nil); err != nil {
					closer.Close()
					return fmt.Errorf("failed to delete old index entry: %w", err)
				}
			}
			closer.Close()
		}

		// Set new index entry
		if err := batch.Set(op.idxKey, []byte(op.docID), nil); err != nil {
			return fmt.Errorf("failed to set index entry: %w", err)
		}

		// Set reverse index
		if err := batch.Set(op.revKey, op.orderKey, nil); err != nil {
			return fmt.Errorf("failed to set reverse index: %w", err)
		}

		// Set map entry
		mapValue := op.pattern + "|" + op.tmplID
		if err := batch.Set(op.mapKey, []byte(mapValue), nil); err != nil {
			return fmt.Errorf("failed to set map entry: %w", err)
		}
	}
	return nil
}

// Upsert inserts or updates a document in the index.
// Uses async batching for better throughput.
func (s *PebbleStore) Upsert(db, pattern, tmplID, docID string, orderKey []byte, progress string) error {
	hash := indexHash(pattern, tmplID)
	deleteKey := db + "|" + hash

	// Check if this index is pending deletion - skip the upsert but still update progress
	s.mu.Lock()
	if _, pendingDelete := s.pendingIndexDeletes[deleteKey]; pendingDelete {
		// Index is being deleted, only update progress if provided
		if progress != "" {
			s.pendingProgress = progress
		}
		s.mu.Unlock()
		return nil
	}

	opKey := pendingOpKey(db, hash, docID)

	// Make a copy of orderKey
	orderKeyCopy := make([]byte, len(orderKey))
	copy(orderKeyCopy, orderKey)

	op := &pendingOp{
		db:       db,
		pattern:  pattern,
		tmplID:   tmplID,
		hash:     hash,
		docID:    docID,
		orderKey: orderKeyCopy,
		idxKey:   indexKey(db, pattern, tmplID, orderKeyCopy),
		revKey:   reverseKey(db, pattern, tmplID, docID),
		mapKey:   mapKey(db, pattern, tmplID),
	}

	s.pending[opKey] = op
	if progress != "" {
		s.pendingProgress = progress
	}
	pendingCount := len(s.pending)
	s.mu.Unlock()

	// Notify batcher if batch size reached
	if pendingCount >= s.batchSize {
		select {
		case s.notifyCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// Delete removes a document from the index.
// Uses async batching for better throughput.
func (s *PebbleStore) Delete(db, pattern, tmplID, docID string, progress string) error {
	hash := indexHash(pattern, tmplID)
	deleteKey := db + "|" + hash

	// Check if this index is pending deletion - skip the delete but still update progress
	s.mu.Lock()
	if _, pendingDelete := s.pendingIndexDeletes[deleteKey]; pendingDelete {
		// Index is being deleted, only update progress if provided
		if progress != "" {
			s.pendingProgress = progress
		}
		s.mu.Unlock()
		return nil
	}

	opKey := pendingOpKey(db, hash, docID)

	op := &pendingOp{
		db:       db,
		pattern:  pattern,
		tmplID:   tmplID,
		hash:     hash,
		docID:    docID,
		orderKey: nil, // nil indicates delete
		revKey:   reverseKey(db, pattern, tmplID, docID),
	}

	s.pending[opKey] = op
	if progress != "" {
		s.pendingProgress = progress
	}
	pendingCount := len(s.pending)
	s.mu.Unlock()

	// Notify batcher if batch size reached
	if pendingCount >= s.batchSize {
		select {
		case s.notifyCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// pendingOpKey returns the key for the pending operations map.
func pendingOpKey(db, hash, docID string) string {
	return db + "|" + hash + "|" + docID
}

// Get returns the OrderKey for a document by ID.
// It checks pending operations first, then falls back to the database.
func (s *PebbleStore) Get(db, pattern, tmplID, docID string) ([]byte, bool) {
	hash := indexHash(pattern, tmplID)
	deleteKey := db + "|" + hash

	// Check if this index is pending deletion
	s.mu.RLock()
	if _, pendingDelete := s.pendingIndexDeletes[deleteKey]; pendingDelete {
		s.mu.RUnlock()
		return nil, false // Index is being deleted
	}

	opKey := pendingOpKey(db, hash, docID)

	// Check pending operations first (newest to oldest)
	// Check pending first (newest), then flushing (older)
	if op, ok := s.pending[opKey]; ok {
		s.mu.RUnlock()
		if op.orderKey == nil {
			return nil, false // Pending delete
		}
		result := make([]byte, len(op.orderKey))
		copy(result, op.orderKey)
		return result, true
	}
	if op, ok := s.flushing[opKey]; ok {
		s.mu.RUnlock()
		if op.orderKey == nil {
			return nil, false // Pending delete
		}
		result := make([]byte, len(op.orderKey))
		copy(result, op.orderKey)
		return result, true
	}
	s.mu.RUnlock()

	// Fall back to database
	revKey := reverseKey(db, pattern, tmplID, docID)
	orderKey, closer, err := s.db.Get(revKey)
	if err != nil {
		return nil, false
	}
	defer closer.Close()

	// Make a copy since the returned slice is only valid until closer.Close()
	result := make([]byte, len(orderKey))
	copy(result, orderKey)
	return result, true
}

// Search returns documents within the specified bounds.
// It merges pending operations with database results.
func (s *PebbleStore) Search(db, pattern, tmplID string, opts store.SearchOptions) ([]store.DocRef, error) {
	hash := indexHash(pattern, tmplID)
	deleteKey := db + "|" + hash

	// Check if this index is pending deletion
	s.mu.RLock()
	if _, pendingDelete := s.pendingIndexDeletes[deleteKey]; pendingDelete {
		s.mu.RUnlock()
		return nil, nil // Index is being deleted, return empty
	}
	s.mu.RUnlock()

	prefix := indexKeyPrefix(db, pattern, tmplID)

	// 1. Snapshot pending operations for this index
	s.mu.RLock()
	memOps := make(map[string]*pendingOp)
	// Copy flushing first (older)
	for _, v := range s.flushing {
		if v.db == db && v.hash == hash {
			memOps[v.docID] = v
		}
	}
	// Copy pending (newer overwrites)
	for _, v := range s.pending {
		if v.db == db && v.hash == hash {
			memOps[v.docID] = v
		}
	}
	s.mu.RUnlock()

	// 2. Build lower and upper bounds
	var lower, upper []byte
	if opts.Lower != nil {
		lower = append(prefix, opts.Lower...)
	} else {
		lower = prefix
	}
	if opts.Upper != nil {
		upper = append(prefix, opts.Upper...)
	} else {
		// Use prefix + 0xFF... as upper bound
		upper = make([]byte, len(prefix))
		copy(upper, prefix)
		upper[len(upper)-1]++
	}

	// Handle StartAfter for pagination
	if opts.StartAfter != nil {
		startAfterKey := append(prefix, opts.StartAfter...)
		if len(lower) == 0 || string(startAfterKey) > string(lower) {
			lower = startAfterKey
		}
	}

	iterOpts := &pebble.IterOptions{
		LowerBound: lower,
		UpperBound: upper,
	}

	iter, err := s.db.NewIter(iterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	limit := opts.Limit
	if limit <= 0 {
		limit = 1000
	}

	// 3. Collect results from database, checking memOps
	seenDocs := make(map[string]bool)
	var results []store.DocRef

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Skip StartAfter cursor itself (exclusive)
		if opts.StartAfter != nil {
			orderKey := parseIndexKey(key, prefix)
			if len(orderKey) > 0 && string(orderKey) == string(opts.StartAfter) {
				continue
			}
		}

		// Extract orderKey from key, docID from value
		orderKey := parseIndexKey(key, prefix)
		docID := string(value)

		// Check if this doc has pending operation
		if op, ok := memOps[docID]; ok {
			if op.orderKey == nil {
				// Pending delete - skip this doc
				seenDocs[docID] = true
				continue
			}
			// Pending update - use pending version instead
			// Check if pending orderKey is in bounds
			if isInBounds(op.orderKey, opts) {
				results = append(results, store.DocRef{
					ID:       docID,
					OrderKey: append([]byte(nil), op.orderKey...),
				})
			}
			seenDocs[docID] = true
			continue
		}

		seenDocs[docID] = true
		results = append(results, store.DocRef{
			ID:       docID,
			OrderKey: append([]byte(nil), orderKey...), // Copy
		})
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	// 4. Add new inserts from memOps not in DB
	for docID, op := range memOps {
		if seenDocs[docID] {
			continue
		}
		if op.orderKey == nil {
			// Delete for non-existent doc - skip
			continue
		}
		// Check if in bounds
		if isInBounds(op.orderKey, opts) {
			results = append(results, store.DocRef{
				ID:       docID,
				OrderKey: append([]byte(nil), op.orderKey...),
			})
		}
	}

	// 5. Sort by orderKey
	sort.Slice(results, func(i, j int) bool {
		return bytes.Compare(results[i].OrderKey, results[j].OrderKey) < 0
	})

	// 6. Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// isInBounds checks if orderKey is within the search bounds.
func isInBounds(orderKey []byte, opts store.SearchOptions) bool {
	if opts.Lower != nil && bytes.Compare(orderKey, opts.Lower) < 0 {
		return false
	}
	if opts.Upper != nil && bytes.Compare(orderKey, opts.Upper) >= 0 {
		return false
	}
	if opts.StartAfter != nil && bytes.Compare(orderKey, opts.StartAfter) <= 0 {
		return false
	}
	return true
}

// DeleteIndex removes all data for an index asynchronously.
// The deletion is queued and processed by the batcher.
func (s *PebbleStore) DeleteIndex(db, pattern, tmplID string) error {
	hash := indexHash(pattern, tmplID)
	deleteKey := db + "|" + hash

	s.mu.Lock()
	// 1. Clear any pending operations for this index
	for key, op := range s.pending {
		if op.db == db && op.hash == hash {
			delete(s.pending, key)
		}
	}

	// 2. Queue the index delete operation
	s.pendingIndexDeletes[deleteKey] = indexDeleteOp{
		db:      db,
		pattern: pattern,
		tmplID:  tmplID,
		hash:    hash,
	}
	s.mu.Unlock()

	// 3. Trigger flush to process the delete
	select {
	case s.notifyCh <- struct{}{}:
	default:
	}

	return nil
}

// deleteByPrefix deletes all keys with the given prefix.
func (s *PebbleStore) deleteByPrefix(prefix []byte) error {
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	upper[len(upper)-1]++

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upper,
	})
	if err != nil {
		return err
	}
	defer iter.Close()

	batch := s.db.NewBatch()
	for iter.First(); iter.Valid(); iter.Next() {
		if err := batch.Delete(iter.Key(), nil); err != nil {
			batch.Close()
			return err
		}
	}

	if err := iter.Error(); err != nil {
		batch.Close()
		return err
	}

	if err := batch.Commit(pebble.Sync); err != nil {
		batch.Close()
		return err
	}

	return batch.Close()
}

// SetState sets the state of an index.
func (s *PebbleStore) SetState(db, pattern, tmplID string, state store.IndexState) error {
	sKey := stateKey(db, pattern, tmplID)
	return s.db.Set(sKey, []byte(state), pebble.Sync)
}

// GetState returns the state of an index.
func (s *PebbleStore) GetState(db, pattern, tmplID string) (store.IndexState, error) {
	sKey := stateKey(db, pattern, tmplID)
	value, closer, err := s.db.Get(sKey)
	if err == pebble.ErrNotFound {
		return store.IndexStateHealthy, nil // Default to healthy
	}
	if err != nil {
		return "", fmt.Errorf("failed to get state: %w", err)
	}
	defer closer.Close()

	return store.IndexState(value), nil
}

// LoadProgress loads the event processing progress.
// Returns the pending progress if available, otherwise reads from disk.
func (s *PebbleStore) LoadProgress() (string, error) {
	// Check pending progress first (most recent)
	s.mu.RLock()
	pendingProg := s.pendingProgress
	s.mu.RUnlock()
	if pendingProg != "" {
		return pendingProg, nil
	}

	// Fall back to persisted progress
	value, closer, err := s.db.Get([]byte(keyProgress))
	if err == pebble.ErrNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to load progress: %w", err)
	}
	defer closer.Close()

	return string(value), nil
}

// ListDatabases returns all database names.
func (s *PebbleStore) ListDatabases() []string {
	dbSet := make(map[string]struct{})

	// 1. Collect from pending operations
	s.mu.RLock()
	for _, op := range s.flushing {
		if op.orderKey != nil { // Only upserts, not deletes
			dbSet[op.db] = struct{}{}
		}
	}
	for _, op := range s.pending {
		if op.orderKey != nil {
			dbSet[op.db] = struct{}{}
		}
	}
	s.mu.RUnlock()

	// 2. Collect from persisted data
	prefix := []byte(prefixMap)
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	upper[len(upper)-1]++

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upper,
	})
	if err != nil {
		s.logger.Error("failed to create iterator for ListDatabases", "error", err)
		// Return what we have from pending
		result := make([]string, 0, len(dbSet))
		for db := range dbSet {
			result = append(result, db)
		}
		return result
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		// Key format: map/{db}/{hash}
		// Skip "map/" prefix (4 chars)
		remainder := key[len(prefix):]
		// Find first '/' to extract db name
		for i, b := range remainder {
			if b == '/' {
				dbName, err := decodePathComponent(string(remainder[:i]))
				if err == nil {
					dbSet[dbName] = struct{}{}
				}
				break
			}
		}
	}

	result := make([]string, 0, len(dbSet))
	for db := range dbSet {
		result = append(result, db)
	}
	return result
}

// ListIndexes returns metadata for all indexes in a database.
func (s *PebbleStore) ListIndexes(db string) []store.IndexInfo {
	// indexKey is pattern + "|" + tmplID
	indexSet := make(map[string]struct {
		pattern string
		tmplID  string
	})

	// 1. Collect from pending operations
	s.mu.RLock()
	for _, op := range s.flushing {
		if op.db == db && op.orderKey != nil { // Only upserts for this db
			key := op.pattern + "|" + op.tmplID
			indexSet[key] = struct {
				pattern string
				tmplID  string
			}{op.pattern, op.tmplID}
		}
	}
	for _, op := range s.pending {
		if op.db == db && op.orderKey != nil {
			key := op.pattern + "|" + op.tmplID
			indexSet[key] = struct {
				pattern string
				tmplID  string
			}{op.pattern, op.tmplID}
		}
	}
	s.mu.RUnlock()

	// 2. Collect from persisted data
	encodedDB := encodePathComponent(db)
	prefix := []byte(prefixMap + encodedDB + "/")
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	upper[len(upper)-1]++

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upper,
	})
	if err != nil {
		s.logger.Error("failed to create iterator for ListIndexes", "error", err)
	} else {
		defer iter.Close()
		for iter.First(); iter.Valid(); iter.Next() {
			value := iter.Value()
			// Value format: {pattern}|{tmplID}
			parts := bytes.SplitN(value, []byte("|"), 2)
			if len(parts) != 2 {
				continue
			}
			pattern := string(parts[0])
			tmplID := string(parts[1])
			key := pattern + "|" + tmplID
			indexSet[key] = struct {
				pattern string
				tmplID  string
			}{pattern, tmplID}
		}
	}

	// 3. Build results, filtering out pending deletes
	s.mu.RLock()
	pendingDeletes := make(map[string]bool)
	for delKey := range s.pendingIndexDeletes {
		pendingDeletes[delKey] = true
	}
	s.mu.RUnlock()

	var results []store.IndexInfo
	for _, idx := range indexSet {
		// Check if this index is pending deletion
		hash := indexHash(idx.pattern, idx.tmplID)
		delKey := db + "|" + hash
		if pendingDeletes[delKey] {
			continue // Skip pending deletes
		}

		// Get state
		state, _ := s.GetState(db, idx.pattern, idx.tmplID)

		// Count documents
		docCount := s.countDocs(db, idx.pattern, idx.tmplID)

		results = append(results, store.IndexInfo{
			Pattern:    idx.pattern,
			TemplateID: idx.tmplID,
			RawPattern: idx.pattern,
			State:      state,
			DocCount:   docCount,
		})
	}

	return results
}

// countDocs counts the number of documents in an index.
func (s *PebbleStore) countDocs(db, pattern, tmplID string) int {
	hash := indexHash(pattern, tmplID)
	docSet := make(map[string]bool) // true = exists, false = deleted

	// 1. Count from pending operations
	s.mu.RLock()
	for _, op := range s.flushing {
		if op.db == db && op.hash == hash {
			docSet[op.docID] = op.orderKey != nil // nil = delete
		}
	}
	for _, op := range s.pending {
		if op.db == db && op.hash == hash {
			docSet[op.docID] = op.orderKey != nil
		}
	}
	s.mu.RUnlock()

	// 2. Count from persisted data
	prefix := reverseKeyPrefix(db, pattern, tmplID)
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	upper[len(upper)-1]++

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upper,
	})
	if err == nil {
		defer iter.Close()
		for iter.First(); iter.Valid(); iter.Next() {
			// Extract docID from key: rev/{db}/{hash}/{docID}
			key := iter.Key()
			docID := string(key[len(prefix):])
			if _, seen := docSet[docID]; !seen {
				docSet[docID] = true
			}
		}
	}

	// 3. Count non-deleted docs
	count := 0
	for _, exists := range docSet {
		if exists {
			count++
		}
	}
	return count
}

// Compile-time check that PebbleStore implements store.Store.
var _ store.Store = (*PebbleStore)(nil)
