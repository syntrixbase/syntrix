// internal/logging/dedup_handler.go
package logging

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

// DedupHandler wraps a slog.Handler and deduplicates identical log entries
// before they get timestamps added. This prevents logs with identical content
// but different timestamps from being treated as different logs.
type DedupHandler struct {
	handler     slog.Handler
	mu          *sync.Mutex // Pointer to allow sharing
	dedupMap    map[uint64]*dedupEntry
	dedupOrder  []uint64
	flushTicker *time.Ticker
	stopChan    chan struct{}
	wg          *sync.WaitGroup // Pointer to allow sharing

	// Config
	batchSize    int
	flushTimeout time.Duration
}

// dedupEntry tracks duplicate log occurrences
type dedupEntry struct {
	record slog.Record
	count  int
}

// DedupHandlerConfig holds configuration for DedupHandler
type DedupHandlerConfig struct {
	// BatchSize is the number of unique entries to accumulate before flushing (default: 100)
	BatchSize int
	// FlushTimeout is the max time to wait before flushing (default: 1s)
	FlushTimeout time.Duration
}

// DefaultDedupHandlerConfig returns default configuration
func DefaultDedupHandlerConfig() DedupHandlerConfig {
	return DedupHandlerConfig{
		BatchSize:    100,
		FlushTimeout: 1 * time.Second,
	}
}

// NewDedupHandler creates a new deduplicating handler with default config
func NewDedupHandler(handler slog.Handler) *DedupHandler {
	return NewDedupHandlerWithConfig(handler, DefaultDedupHandlerConfig())
}

// NewDedupHandlerWithConfig creates a new deduplicating handler with custom config
func NewDedupHandlerWithConfig(handler slog.Handler, cfg DedupHandlerConfig) *DedupHandler {
	dh := &DedupHandler{
		handler:      handler,
		mu:           &sync.Mutex{}, // Allocate mutex pointer
		dedupMap:     make(map[uint64]*dedupEntry),
		dedupOrder:   make([]uint64, 0, cfg.BatchSize),
		flushTicker:  time.NewTicker(cfg.FlushTimeout),
		stopChan:     make(chan struct{}),
		wg:           &sync.WaitGroup{}, // Allocate waitgroup pointer
		batchSize:    cfg.BatchSize,
		flushTimeout: cfg.FlushTimeout,
	}

	dh.wg.Add(1)
	go dh.flushLoop()

	return dh
}

// Enabled reports whether the handler handles records at the given level.
func (h *DedupHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle deduplicates and buffers log records
func (h *DedupHandler) Handle(ctx context.Context, r slog.Record) error {
	// Compute hash of record content (excluding time)
	key := h.hashRecord(r)

	h.mu.Lock()
	defer h.mu.Unlock()

	if entry, exists := h.dedupMap[key]; exists {
		// Duplicate found, increment count
		entry.count++
	} else {
		// New entry
		h.dedupMap[key] = &dedupEntry{
			record: r.Clone(),
			count:  1,
		}
		h.dedupOrder = append(h.dedupOrder, key)

		// Flush if batch is full
		if len(h.dedupOrder) >= h.batchSize {
			h.flushBatch()
		}
	}

	return nil
}

// hashRecord computes a hash of the record content (level, message, attributes)
// excluding the timestamp
func (h *DedupHandler) hashRecord(r slog.Record) uint64 {
	hash := xxhash.New()

	// Hash level
	hash.WriteString(r.Level.String())
	hash.WriteString("|")

	// Hash message
	hash.WriteString(r.Message)
	hash.WriteString("|")

	// Hash attributes
	r.Attrs(func(a slog.Attr) bool {
		hash.WriteString(a.Key)
		hash.WriteString("=")
		hash.WriteString(a.Value.String())
		hash.WriteString("|")
		return true
	})

	return hash.Sum64()
}

// flushLoop periodically flushes the deduplicated logs
func (h *DedupHandler) flushLoop() {
	defer h.wg.Done()

	for {
		select {
		case <-h.flushTicker.C:
			h.mu.Lock()
			if len(h.dedupOrder) > 0 {
				h.flushBatch()
			}
			h.mu.Unlock()

		case <-h.stopChan:
			h.mu.Lock()
			if len(h.dedupOrder) > 0 {
				h.flushBatch()
			}
			h.mu.Unlock()
			return
		}
	}
}

// flushBatch writes all deduplicated entries to the underlying handler
// Must be called with h.mu locked
func (h *DedupHandler) flushBatch() {
	if len(h.dedupOrder) == 0 {
		return
	}

	// Collect all records to flush while holding the lock
	records := make([]slog.Record, 0, len(h.dedupOrder))
	for _, key := range h.dedupOrder {
		entry := h.dedupMap[key]

		// Safety check: entry might be nil in rare race conditions
		if entry == nil {
			continue
		}

		// Clone the record to avoid modifying the original
		r := entry.record

		if entry.count > 1 {
			// Add repeat count as an attribute
			r.AddAttrs(slog.Int("repeated_count", entry.count))
		}

		records = append(records, r)
	}

	// Clear the batch before releasing the lock
	h.dedupMap = make(map[uint64]*dedupEntry)
	h.dedupOrder = h.dedupOrder[:0]

	// Release the lock before calling the underlying handler
	// This prevents deadlock if the handler logs anything
	h.mu.Unlock()

	// Send all records to underlying handler (this adds the timestamp)
	for _, r := range records {
		_ = h.handler.Handle(context.Background(), r)
	}

	// Re-acquire the lock before returning
	h.mu.Lock()
}

// WithAttrs returns a new handler with additional attributes.
// We wrap the underlying handler's WithAttrs, but keep using the same
// DedupHandler to maintain shared deduplication state.
func (h *DedupHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Wrap the underlying handler's WithAttrs result
	// This keeps the deduplication at the top level
	newHandler := h.handler.WithAttrs(attrs)

	// Return a new DedupHandler that shares ALL state with the original
	// including the mutex, maps, and goroutine
	return &DedupHandler{
		handler:      newHandler,
		mu:           h.mu,          // Share mutex
		dedupMap:     h.dedupMap,    // Share dedup state
		dedupOrder:   h.dedupOrder,  // Share order
		flushTicker:  h.flushTicker, // Share ticker
		stopChan:     h.stopChan,    // Share stop channel
		wg:           h.wg,          // Share wait group
		batchSize:    h.batchSize,
		flushTimeout: h.flushTimeout,
	}
}

// WithGroup returns a new handler with a group name appended.
func (h *DedupHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	// Wrap the underlying handler's WithGroup result
	newHandler := h.handler.WithGroup(name)

	// Return a new DedupHandler that shares ALL state with the original
	return &DedupHandler{
		handler:      newHandler,
		mu:           h.mu,          // Share mutex
		dedupMap:     h.dedupMap,    // Share dedup state
		dedupOrder:   h.dedupOrder,  // Share order
		flushTicker:  h.flushTicker, // Share ticker
		stopChan:     h.stopChan,    // Share stop channel
		wg:           h.wg,          // Share wait group
		batchSize:    h.batchSize,
		flushTimeout: h.flushTimeout,
	}
}

// Close gracefully shuts down the deduplicating handler
func (h *DedupHandler) Close() error {
	close(h.stopChan)
	h.flushTicker.Stop()
	h.wg.Wait()
	return nil
}
