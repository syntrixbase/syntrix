package indexer_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/syntrixbase/syntrix/internal/indexer"
	"github.com/syntrixbase/syntrix/internal/puller"
	"github.com/syntrixbase/syntrix/internal/storage"
)

// mockPuller implements puller.Service for testing.
type mockPuller struct {
	events chan *puller.Event
}

func newMockPuller(bufferSize int) *mockPuller {
	return &mockPuller{
		events: make(chan *puller.Event, bufferSize),
	}
}

func (m *mockPuller) Subscribe(ctx context.Context, consumerID string, after string) <-chan *puller.Event {
	return m.events
}

func (m *mockPuller) pushEvent(evt *puller.ChangeEvent, progress string) {
	m.events <- &puller.Event{
		Change:   evt,
		Progress: progress,
	}
}

func (m *mockPuller) close() {
	close(m.events)
}

// createTestEvent creates a change event for testing.
func createTestEvent(database, collection, docID string, data map[string]any) *puller.ChangeEvent {
	return &puller.ChangeEvent{
		EventID:    "evt-" + docID,
		DatabaseID: database,
		OpType:     puller.OperationInsert,
		FullDocument: &storage.StoredDoc{
			Id:         docID,
			DatabaseID: database,
			Collection: collection,
			Data:       data,
		},
		ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: 1},
		Timestamp:   time.Now().UnixMilli(),
	}
}

// templateYAML defines test templates.
const templateYAML = `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc
`

// setupIndexerService creates and starts an indexer service for testing.
func setupIndexerService(t *testing.T, mockPullerSvc *mockPuller) (indexer.LocalService, context.Context, context.CancelFunc) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create temp file for templates
	tmpFile, err := os.CreateTemp("", "templates-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	if _, err := tmpFile.WriteString(templateYAML); err != nil {
		t.Fatalf("failed to write templates: %v", err)
	}
	tmpFile.Close()

	cfg := indexer.Config{
		TemplatePath: tmpFile.Name(),
		ConsumerID:   "test-indexer",
	}
	svc := indexer.NewService(cfg, mockPullerSvc, logger)

	ctx, cancel := context.WithCancel(context.Background())

	if err := svc.Start(ctx); err != nil {
		cancel()
		t.Fatalf("failed to start service: %v", err)
	}

	return svc, ctx, cancel
}

// stopService stops the indexer service gracefully.
func stopService(t *testing.T, svc indexer.LocalService, mockPullerSvc *mockPuller) {
	t.Helper()
	mockPullerSvc.close()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := svc.Stop(stopCtx); err != nil {
		t.Errorf("failed to stop service: %v", err)
	}
}

func TestIntegration_LargeDataset(t *testing.T) {
	t.Parallel()
	const docCount = 1000
	mockPullerSvc := newMockPuller(docCount + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Generate 1000 documents with random timestamps
	type docData struct {
		id        string
		timestamp int64
	}
	docs := make([]docData, docCount)
	for i := 0; i < docCount; i++ {
		docs[i] = docData{
			id:        fmt.Sprintf("user%04d", i),
			timestamp: int64(docCount - i), // Reverse order so we can verify sorting
		}
	}

	// Push all events
	for i, doc := range docs {
		evt := createTestEvent("db1", "users", doc.id, map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": doc.timestamp,
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	// Wait for events to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify stats
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.EventsApplied != docCount {
		t.Errorf("expected %d events applied, got %d", docCount, stats.EventsApplied)
	}

	// Search with limit
	plan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 100,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 100 {
		t.Fatalf("expected 100 results, got %d", len(results))
	}

	// Verify first 100 results are ordered correctly (highest timestamp first)
	// user0000 has timestamp 1000, user0001 has 999, etc.
	for i, ref := range results {
		expected := fmt.Sprintf("user%04d", i)
		if ref.ID != expected {
			t.Errorf("result[%d]: expected %s, got %s", i, expected, ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_ConcurrentUpdates(t *testing.T) {
	t.Parallel()
	const docCount = 100
	const updateRounds = 5
	mockPullerSvc := newMockPuller(docCount*(updateRounds+1) + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert initial documents
	for i := 0; i < docCount; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i * 10),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("insert-%d", i))
	}

	time.Sleep(50 * time.Millisecond)

	// Update documents multiple times
	for round := 0; round < updateRounds; round++ {
		for i := 0; i < docCount; i++ {
			updateEvt := &puller.ChangeEvent{
				EventID:    fmt.Sprintf("evt-update-%d-%d", round, i),
				DatabaseID: "db1",
				OpType:     puller.OperationUpdate,
				FullDocument: &storage.StoredDoc{
					Id:         fmt.Sprintf("user%03d", i),
					DatabaseID: "db1",
					Collection: "users",
					Data: map[string]any{
						"name":      fmt.Sprintf("User %d (v%d)", i, round+2),
						"timestamp": int64((round+1)*docCount + i),
					},
				},
				ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: uint32(round*docCount + i)},
				Timestamp:   time.Now().UnixMilli(),
			}
			mockPullerSvc.pushEvent(updateEvt, fmt.Sprintf("update-%d-%d", round, i))
		}
	}

	time.Sleep(200 * time.Millisecond)

	// Verify final order - after all updates, the order should be based on final timestamps
	plan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: docCount,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != docCount {
		t.Fatalf("expected %d results, got %d", docCount, len(results))
	}

	// Final timestamps are: updateRounds*docCount + i for each user
	// So user099 has highest timestamp: 5*100 + 99 = 599
	// user000 has lowest timestamp: 5*100 + 0 = 500
	for i, ref := range results {
		expected := fmt.Sprintf("user%03d", docCount-1-i)
		if ref.ID != expected {
			t.Errorf("result[%d]: expected %s, got %s", i, expected, ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_MultipleCollections(t *testing.T) {
	t.Parallel()
	const usersCount = 200
	const ordersCount = 150
	const productsCount = 100
	totalEvents := usersCount + ordersCount + productsCount
	mockPullerSvc := newMockPuller(totalEvents + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert users
	for i := 0; i < usersCount; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("user-%d", i))
	}

	// Insert orders
	for i := 0; i < ordersCount; i++ {
		evt := createTestEvent("db1", "orders", fmt.Sprintf("order%03d", i), map[string]any{
			"customer": fmt.Sprintf("user%03d", i%usersCount),
			"amount":   float64(i * 100),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("order-%d", i))
	}

	// Insert products
	for i := 0; i < productsCount; i++ {
		evt := createTestEvent("db1", "products", fmt.Sprintf("prod%03d", i), map[string]any{
			"name":  fmt.Sprintf("Product %d", i),
			"price": float64(i * 10),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("product-%d", i))
	}

	time.Sleep(200 * time.Millisecond)

	// Verify stats
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.EventsApplied != int64(totalEvents) {
		t.Errorf("expected %d events applied, got %d", totalEvents, stats.EventsApplied)
	}

	// Search users (ordered by timestamp desc)
	userPlan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      50,
	}
	userResults, err := svc.Search(ctx, "db1", userPlan)
	if err != nil {
		t.Fatalf("search users failed: %v", err)
	}
	if len(userResults) != 50 {
		t.Fatalf("expected 50 user results, got %d", len(userResults))
	}
	// Highest timestamp is 199, so first result should be user199
	if userResults[0].ID != "user199" {
		t.Errorf("expected user199, got %s", userResults[0].ID)
	}

	// Search orders (ordered by amount desc)
	orderPlan := indexer.Plan{
		Collection: "orders",
		OrderBy:    []indexer.OrderField{{Field: "amount", Direction: indexer.Desc}},
		Limit:      50,
	}
	orderResults, err := svc.Search(ctx, "db1", orderPlan)
	if err != nil {
		t.Fatalf("search orders failed: %v", err)
	}
	if len(orderResults) != 50 {
		t.Fatalf("expected 50 order results, got %d", len(orderResults))
	}
	// Highest amount is 14900 (149 * 100), so first result should be order149
	if orderResults[0].ID != "order149" {
		t.Errorf("expected order149, got %s", orderResults[0].ID)
	}

	// Search products (ordered by price asc)
	productPlan := indexer.Plan{
		Collection: "products",
		OrderBy:    []indexer.OrderField{{Field: "price", Direction: indexer.Asc}},
		Limit:      50,
	}
	productResults, err := svc.Search(ctx, "db1", productPlan)
	if err != nil {
		t.Fatalf("search products failed: %v", err)
	}
	if len(productResults) != 50 {
		t.Fatalf("expected 50 product results, got %d", len(productResults))
	}
	// Lowest price is 0, so first result should be prod000
	if productResults[0].ID != "prod000" {
		t.Errorf("expected prod000, got %s", productResults[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_BulkDeletes(t *testing.T) {
	t.Parallel()
	const docCount = 200
	mockPullerSvc := newMockPuller(docCount*2 + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert documents
	for i := 0; i < docCount; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("insert-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Verify all documents are indexed
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      docCount,
	}
	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("initial search failed: %v", err)
	}
	if len(results) != docCount {
		t.Fatalf("expected %d results before delete, got %d", docCount, len(results))
	}

	// Delete half the documents (even numbers)
	for i := 0; i < docCount; i += 2 {
		deleteEvt := &puller.ChangeEvent{
			EventID:    fmt.Sprintf("evt-delete-%d", i),
			DatabaseID: "db1",
			OpType:     puller.OperationUpdate,
			FullDocument: &storage.StoredDoc{
				Id:         fmt.Sprintf("user%03d", i),
				DatabaseID: "db1",
				Collection: "users",
				Data: map[string]any{
					"name":      fmt.Sprintf("User %d", i),
					"timestamp": int64(i),
				},
				Deleted: true,
			},
			ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: uint32(i)},
			Timestamp:   time.Now().UnixMilli(),
		}
		mockPullerSvc.pushEvent(deleteEvt, fmt.Sprintf("delete-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Verify only odd-numbered documents remain
	results, err = svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search after delete failed: %v", err)
	}
	expectedCount := docCount / 2
	if len(results) != expectedCount {
		t.Fatalf("expected %d results after delete, got %d", expectedCount, len(results))
	}

	// All results should be odd-numbered users
	for _, ref := range results {
		var num int
		fmt.Sscanf(ref.ID, "user%d", &num)
		if num%2 == 0 {
			t.Errorf("found deleted user in results: %s", ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_PatternWithManyUsers(t *testing.T) {
	t.Parallel()
	const userCount = 20
	const chatsPerUser = 50
	totalEvents := userCount * chatsPerUser
	mockPullerSvc := newMockPuller(totalEvents + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Create chats for many users
	for u := 0; u < userCount; u++ {
		for c := 0; c < chatsPerUser; c++ {
			evt := createTestEvent("db1",
				fmt.Sprintf("users/user%02d/chats", u),
				fmt.Sprintf("chat-u%02d-c%03d", u, c),
				map[string]any{
					"priority":  int64(c % 5),      // Priority 0-4
					"timestamp": int64(u*1000 + c), // Unique timestamp
				})
			mockPullerSvc.pushEvent(evt, fmt.Sprintf("chat-%d-%d", u, c))
		}
	}

	time.Sleep(200 * time.Millisecond)

	// Verify stats
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.EventsApplied != int64(totalEvents) {
		t.Errorf("expected %d events applied, got %d", totalEvents, stats.EventsApplied)
	}

	// Search - all chats from all users are in the same index
	plan := indexer.Plan{
		Collection: "users/user00/chats", // Any user works, pattern matches all
		OrderBy: []indexer.OrderField{
			{Field: "priority", Direction: indexer.Desc},
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 100,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 100 {
		t.Fatalf("expected 100 results, got %d", len(results))
	}

	// Verify ordering: priority desc, then timestamp desc
	// All priority=4 chats should come first (c=4,9,14,19,24,29,34,39,44,49 for each user)
	// Within same priority, higher timestamp comes first
	lastPriority := int64(5) // Start higher than max
	lastTimestamp := int64(999999)
	for i, ref := range results {
		// Extract priority and timestamp from chat ID
		var u, c int
		fmt.Sscanf(ref.ID, "chat-u%02d-c%03d", &u, &c)
		priority := int64(c % 5)
		timestamp := int64(u*1000 + c)

		if priority > lastPriority {
			t.Errorf("result[%d]: priority %d > previous %d", i, priority, lastPriority)
		} else if priority == lastPriority && timestamp > lastTimestamp {
			t.Errorf("result[%d]: timestamp %d > previous %d with same priority", i, timestamp, lastTimestamp)
		}
		lastPriority = priority
		lastTimestamp = timestamp
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_MultiDatabaseLargeScale(t *testing.T) {
	t.Parallel()
	const dbCount = 5
	const docsPerDB = 100
	totalEvents := dbCount * docsPerDB
	mockPullerSvc := newMockPuller(totalEvents + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert documents across multiple databases
	for db := 0; db < dbCount; db++ {
		for i := 0; i < docsPerDB; i++ {
			evt := createTestEvent(
				fmt.Sprintf("db%d", db),
				"users",
				fmt.Sprintf("user%03d", i),
				map[string]any{
					"name":      fmt.Sprintf("User %d in DB%d", i, db),
					"timestamp": int64(db*1000 + i),
				})
			mockPullerSvc.pushEvent(evt, fmt.Sprintf("db%d-user%d", db, i))
		}
	}

	time.Sleep(200 * time.Millisecond)

	// Verify stats
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.EventsApplied != int64(totalEvents) {
		t.Errorf("expected %d events applied, got %d", totalEvents, stats.EventsApplied)
	}

	// Search each database
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      docsPerDB,
	}

	for db := 0; db < dbCount; db++ {
		results, err := svc.Search(ctx, fmt.Sprintf("db%d", db), plan)
		if err != nil {
			t.Fatalf("search db%d failed: %v", db, err)
		}
		if len(results) != docsPerDB {
			t.Fatalf("db%d: expected %d results, got %d", db, docsPerDB, len(results))
		}

		// Verify first result is user099 (highest timestamp in this db)
		if results[0].ID != "user099" {
			t.Errorf("db%d: expected user099, got %s", db, results[0].ID)
		}

		// Verify last result is user000 (lowest timestamp in this db)
		if results[docsPerDB-1].ID != "user000" {
			t.Errorf("db%d: expected user000, got %s", db, results[docsPerDB-1].ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_MixedOperations(t *testing.T) {
	t.Parallel()
	const initialDocs = 100
	mockPullerSvc := newMockPuller(500)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Phase 1: Insert initial documents
	for i := 0; i < initialDocs; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("insert-%d", i))
	}

	time.Sleep(200 * time.Millisecond)

	// Phase 2: Mixed operations - update some, delete some, insert new
	// Update users 0-19 (bump their timestamps)
	for i := 0; i < 20; i++ {
		updateEvt := &puller.ChangeEvent{
			EventID:    fmt.Sprintf("evt-update-%d", i),
			DatabaseID: "db1",
			OpType:     puller.OperationUpdate,
			FullDocument: &storage.StoredDoc{
				Id:         fmt.Sprintf("user%03d", i),
				DatabaseID: "db1",
				Collection: "users",
				Data: map[string]any{
					"name":      fmt.Sprintf("User %d (Updated)", i),
					"timestamp": int64(1000 + i), // Bump to top
				},
			},
			ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: uint32(i)},
			Timestamp:   time.Now().UnixMilli(),
		}
		mockPullerSvc.pushEvent(updateEvt, fmt.Sprintf("update-%d", i))
	}

	// Delete users 80-99
	for i := 80; i < 100; i++ {
		deleteEvt := &puller.ChangeEvent{
			EventID:    fmt.Sprintf("evt-delete-%d", i),
			DatabaseID: "db1",
			OpType:     puller.OperationUpdate,
			FullDocument: &storage.StoredDoc{
				Id:         fmt.Sprintf("user%03d", i),
				DatabaseID: "db1",
				Collection: "users",
				Data:       map[string]any{"timestamp": int64(i)},
				Deleted:    true,
			},
			ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: uint32(100 + i)},
			Timestamp:   time.Now().UnixMilli(),
		}
		mockPullerSvc.pushEvent(deleteEvt, fmt.Sprintf("delete-%d", i))
	}

	// Insert new users 100-119
	for i := 100; i < 120; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(500 + i), // Mid-range timestamps
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("new-insert-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Verify final state
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      200,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// Expected count: 100 - 20 (deleted) + 20 (new) = 100
	expectedCount := 100
	if len(results) != expectedCount {
		t.Fatalf("expected %d results, got %d", expectedCount, len(results))
	}

	// Verify updated users (0-19) are at the top
	// Their timestamps are 1000-1019, so they should be first 20 results
	for i := 0; i < 20; i++ {
		expected := fmt.Sprintf("user%03d", 19-i) // 1019, 1018, ..., 1000
		if results[i].ID != expected {
			t.Errorf("result[%d]: expected %s, got %s", i, expected, results[i].ID)
		}
	}

	// Verify deleted users (80-99) are not in results
	resultIDs := make(map[string]bool)
	for _, ref := range results {
		resultIDs[ref.ID] = true
	}
	for i := 80; i < 100; i++ {
		deletedID := fmt.Sprintf("user%03d", i)
		if resultIDs[deletedID] {
			t.Errorf("deleted user %s found in results", deletedID)
		}
	}

	// Verify new users (100-119) are in results
	for i := 100; i < 120; i++ {
		newID := fmt.Sprintf("user%03d", i)
		if !resultIDs[newID] {
			t.Errorf("new user %s not found in results", newID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_EventIndexing(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Push some events
	events := []*puller.ChangeEvent{
		createTestEvent("db1", "users", "user1", map[string]any{
			"name":      "Alice",
			"timestamp": int64(1000),
		}),
		createTestEvent("db1", "users", "user2", map[string]any{
			"name":      "Bob",
			"timestamp": int64(2000),
		}),
		createTestEvent("db1", "users", "user3", map[string]any{
			"name":      "Charlie",
			"timestamp": int64(1500),
		}),
	}

	for i, evt := range events {
		mockPullerSvc.pushEvent(evt, "progress-"+string(rune('a'+i)))
	}

	// Wait for events to be processed
	time.Sleep(50 * time.Millisecond)

	// Verify stats
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.EventsApplied != 3 {
		t.Errorf("expected 3 events applied, got %d", stats.EventsApplied)
	}

	// Search for users ordered by timestamp desc
	plan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results should be ordered by timestamp desc: user2 (2000), user3 (1500), user1 (1000)
	expectedOrder := []string{"user2", "user3", "user1"}
	for i, ref := range results {
		if ref.ID != expectedOrder[i] {
			t.Errorf("result[%d]: expected %s, got %s", i, expectedOrder[i], ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_PatternMatching(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Push events for pattern "users/*/chats"
	events := []*puller.ChangeEvent{
		createTestEvent("db1", "users/alice/chats", "chat1", map[string]any{
			"priority":  int64(1),
			"timestamp": int64(100),
		}),
		createTestEvent("db1", "users/alice/chats", "chat2", map[string]any{
			"priority":  int64(3),
			"timestamp": int64(200),
		}),
		createTestEvent("db1", "users/bob/chats", "chat3", map[string]any{
			"priority":  int64(2),
			"timestamp": int64(150),
		}),
	}

	for i, evt := range events {
		mockPullerSvc.pushEvent(evt, "p-"+string(rune('a'+i)))
	}

	time.Sleep(50 * time.Millisecond)

	// Search for chats - the pattern "users/{userId}/chats" matches all users' chats.
	// All documents matching the pattern are stored in the same index.
	// When searching for "users/alice/chats", we get all chats from the pattern index.
	plan := indexer.Plan{
		Collection: "users/alice/chats",
		OrderBy: []indexer.OrderField{
			{Field: "priority", Direction: indexer.Desc},
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// All chats from the pattern index are returned, ordered by priority desc, then timestamp desc.
	// chat2 (priority=3), chat3 (priority=2), chat1 (priority=1)
	if len(results) != 3 {
		t.Fatalf("expected 3 results for pattern-matched chats, got %d", len(results))
	}

	expectedOrder := []string{"chat2", "chat3", "chat1"}
	for i, ref := range results {
		if ref.ID != expectedOrder[i] {
			t.Errorf("result[%d]: expected %s, got %s", i, expectedOrder[i], ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_NoMatchingIndex(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Search for a collection that has no matching template
	plan := indexer.Plan{
		Collection: "nonexistent",
		OrderBy: []indexer.OrderField{
			{Field: "createdAt", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	_, err := svc.Search(ctx, "db1", plan)
	if err != indexer.ErrNoMatchingIndex {
		t.Errorf("expected ErrNoMatchingIndex, got %v", err)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_DocumentUpdate(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert a document
	mockPullerSvc.pushEvent(createTestEvent("db1", "users", "user1", map[string]any{
		"name":      "Alice",
		"timestamp": int64(1000),
	}), "p1")

	time.Sleep(50 * time.Millisecond)

	// Update the document with a new timestamp
	updateEvt := &puller.ChangeEvent{
		EventID:    "evt-update1",
		DatabaseID: "db1",
		OpType:     puller.OperationUpdate,
		FullDocument: &storage.StoredDoc{
			Id:         "user1",
			DatabaseID: "db1",
			Collection: "users",
			Data: map[string]any{
				"name":      "Alice Updated",
				"timestamp": int64(3000), // Higher timestamp, should move to first position
			},
		},
		ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: 2},
		Timestamp:   time.Now().UnixMilli(),
	}
	mockPullerSvc.pushEvent(updateEvt, "p2")

	// Insert another document
	mockPullerSvc.pushEvent(createTestEvent("db1", "users", "user2", map[string]any{
		"name":      "Bob",
		"timestamp": int64(2000),
	}), "p3")

	time.Sleep(50 * time.Millisecond)

	// Search - user1 should now be first due to higher timestamp
	plan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// After update: user1 (3000), user2 (2000)
	expectedOrder := []string{"user1", "user2"}
	for i, ref := range results {
		if ref.ID != expectedOrder[i] {
			t.Errorf("result[%d]: expected %s, got %s", i, expectedOrder[i], ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_DocumentDelete(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert two documents
	mockPullerSvc.pushEvent(createTestEvent("db1", "users", "user1", map[string]any{
		"name":      "Alice",
		"timestamp": int64(1000),
	}), "p1")

	mockPullerSvc.pushEvent(createTestEvent("db1", "users", "user2", map[string]any{
		"name":      "Bob",
		"timestamp": int64(2000),
	}), "p2")

	time.Sleep(50 * time.Millisecond)

	// Delete user2 (soft delete)
	deleteEvt := &puller.ChangeEvent{
		EventID:    "evt-delete1",
		DatabaseID: "db1",
		OpType:     puller.OperationUpdate,
		FullDocument: &storage.StoredDoc{
			Id:         "user2",
			DatabaseID: "db1",
			Collection: "users",
			Data: map[string]any{
				"name":      "Bob",
				"timestamp": int64(2000),
			},
			Deleted: true, // Soft deleted
		},
		ClusterTime: puller.ClusterTime{T: uint32(time.Now().Unix()), I: 3},
		Timestamp:   time.Now().UnixMilli(),
	}
	mockPullerSvc.pushEvent(deleteEvt, "p3")

	time.Sleep(50 * time.Millisecond)

	// Search - only user1 should remain
	plan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result after delete, got %d", len(results))
	}

	if results[0].ID != "user1" {
		t.Errorf("expected user1, got %s", results[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_MultipleDatabase(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert documents into two different databases
	mockPullerSvc.pushEvent(createTestEvent("db1", "users", "user1", map[string]any{
		"name":      "Alice-DB1",
		"timestamp": int64(1000),
	}), "p1")

	mockPullerSvc.pushEvent(createTestEvent("db2", "users", "user1", map[string]any{
		"name":      "Alice-DB2",
		"timestamp": int64(2000),
	}), "p2")

	mockPullerSvc.pushEvent(createTestEvent("db1", "users", "user2", map[string]any{
		"name":      "Bob-DB1",
		"timestamp": int64(3000),
	}), "p3")

	time.Sleep(50 * time.Millisecond)

	plan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	// Search db1
	results1, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search db1 failed: %v", err)
	}

	if len(results1) != 2 {
		t.Fatalf("expected 2 results for db1, got %d", len(results1))
	}

	// db1: user2 (3000), user1 (1000)
	if results1[0].ID != "user2" || results1[1].ID != "user1" {
		t.Errorf("db1 results order incorrect: %v, %v", results1[0].ID, results1[1].ID)
	}

	// Search db2
	results2, err := svc.Search(ctx, "db2", plan)
	if err != nil {
		t.Fatalf("search db2 failed: %v", err)
	}

	if len(results2) != 1 {
		t.Fatalf("expected 1 result for db2, got %d", len(results2))
	}

	if results2[0].ID != "user1" {
		t.Errorf("expected user1 in db2, got %s", results2[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_Pagination(t *testing.T) {
	t.Parallel()
	const docCount = 50
	mockPullerSvc := newMockPuller(docCount + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert documents
	for i := 0; i < docCount; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(docCount - i), // Reverse order for desc sorting
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(200 * time.Millisecond)

	// Fetch first page
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      10,
	}

	page1, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(page1) != 10 {
		t.Fatalf("expected 10 results in page 1, got %d", len(page1))
	}

	// Collect all results using pagination
	allResults := make([]string, 0, docCount)
	for _, ref := range page1 {
		allResults = append(allResults, ref.ID)
	}

	// Fetch remaining pages
	cursor := page1[len(page1)-1].OrderKey
	for len(allResults) < docCount {
		plan.StartAfter = base64.RawURLEncoding.EncodeToString(cursor)
		page, err := svc.Search(ctx, "db1", plan)
		if err != nil {
			t.Fatalf("pagination failed: %v", err)
		}
		if len(page) == 0 {
			break
		}
		for _, ref := range page {
			allResults = append(allResults, ref.ID)
		}
		cursor = page[len(page)-1].OrderKey
	}

	if len(allResults) != docCount {
		t.Fatalf("expected %d total results, got %d", docCount, len(allResults))
	}

	// Verify all unique and in order
	seen := make(map[string]bool)
	for i, id := range allResults {
		if seen[id] {
			t.Errorf("duplicate ID found: %s", id)
		}
		seen[id] = true

		// Verify order: user000 has highest timestamp (50), user049 has lowest (1)
		expected := fmt.Sprintf("user%03d", i)
		if id != expected {
			t.Errorf("result[%d]: expected %s, got %s", i, expected, id)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_HealthCheck(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Check health when running
	health, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if health.Status != indexer.HealthOK {
		t.Errorf("expected HealthOK, got %s", health.Status)
	}

	// Insert some events and check again
	for i := 0; i < 10; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%d", i), map[string]any{
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(50 * time.Millisecond)

	health, err = svc.Health(ctx)
	if err != nil {
		t.Fatalf("health check after events failed: %v", err)
	}

	if health.Status != indexer.HealthOK {
		t.Errorf("expected HealthOK after events, got %s", health.Status)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_StatsAccumulation(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(500)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Initial stats
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("initial stats failed: %v", err)
	}
	if stats.EventsApplied != 0 {
		t.Errorf("expected 0 events initially, got %d", stats.EventsApplied)
	}

	// Insert events in batches and verify stats accumulation
	batches := []int{10, 25, 50, 15}
	totalEvents := int64(0)

	for _, batchSize := range batches {
		for i := 0; i < batchSize; i++ {
			evt := createTestEvent("db1", "users", fmt.Sprintf("user-%d-%d", len(batches), i), map[string]any{
				"timestamp": int64(i),
			})
			mockPullerSvc.pushEvent(evt, fmt.Sprintf("batch-%d", totalEvents))
			totalEvents++
		}

		time.Sleep(50 * time.Millisecond)

		stats, err = svc.Stats(ctx)
		if err != nil {
			t.Fatalf("stats after batch failed: %v", err)
		}
		if stats.EventsApplied != totalEvents {
			t.Errorf("expected %d events after batch, got %d", totalEvents, stats.EventsApplied)
		}
	}

	if stats.EventsApplied != 100 {
		t.Errorf("expected 100 total events, got %d", stats.EventsApplied)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_AscendingOrder(t *testing.T) {
	t.Parallel()
	const docCount = 50
	mockPullerSvc := newMockPuller(docCount + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert products with prices
	for i := 0; i < docCount; i++ {
		evt := createTestEvent("db1", "products", fmt.Sprintf("prod%03d", i), map[string]any{
			"name":  fmt.Sprintf("Product %d", i),
			"price": float64(i * 10),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(200 * time.Millisecond)

	// Search products ordered by price ascending
	plan := indexer.Plan{
		Collection: "products",
		OrderBy:    []indexer.OrderField{{Field: "price", Direction: indexer.Asc}},
		Limit:      docCount,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != docCount {
		t.Fatalf("expected %d results, got %d", docCount, len(results))
	}

	// Verify ascending order: prod000 (price=0) should be first
	for i, ref := range results {
		expected := fmt.Sprintf("prod%03d", i)
		if ref.ID != expected {
			t.Errorf("result[%d]: expected %s, got %s", i, expected, ref.ID)
		}
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_DuplicateEventIdempotency(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert the same document multiple times (simulating redelivery)
	for i := 0; i < 5; i++ {
		evt := createTestEvent("db1", "users", "user001", map[string]any{
			"name":      "Alice",
			"timestamp": int64(1000 + i), // Different timestamp each time
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(50 * time.Millisecond)

	// Search - should only have one document
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      10,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (deduped), got %d", len(results))
	}

	if results[0].ID != "user001" {
		t.Errorf("expected user001, got %s", results[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_EmptyCollection(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert some users but not orders
	for i := 0; i < 5; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%d", i), map[string]any{
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(50 * time.Millisecond)

	// Search orders - template exists but no documents have been indexed yet
	// This returns ErrIndexNotReady because the index hasn't been created
	plan := indexer.Plan{
		Collection: "orders",
		OrderBy:    []indexer.OrderField{{Field: "amount", Direction: indexer.Desc}},
		Limit:      10,
	}

	_, err := svc.Search(ctx, "db1", plan)
	if err != indexer.ErrIndexNotReady {
		t.Errorf("expected ErrIndexNotReady for collection with no indexed documents, got %v", err)
	}

	// Now insert one order to create the index
	evt := createTestEvent("db1", "orders", "order001", map[string]any{
		"amount": float64(100),
	})
	mockPullerSvc.pushEvent(evt, "order-1")

	time.Sleep(50 * time.Millisecond)

	// Search again - should now work and return the one document
	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed after inserting order: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result after inserting order, got %d", len(results))
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_LargeDocumentBatch(t *testing.T) {
	t.Parallel()
	const docCount = 5000
	mockPullerSvc := newMockPuller(docCount + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert 5000 documents rapidly
	for i := 0; i < docCount; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%05d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Verify all events processed
	stats, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}
	if stats.EventsApplied != docCount {
		t.Errorf("expected %d events, got %d", docCount, stats.EventsApplied)
	}

	// Verify search works correctly
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      100,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 100 {
		t.Fatalf("expected 100 results, got %d", len(results))
	}

	// First result should be user04999 (highest timestamp)
	if results[0].ID != "user04999" {
		t.Errorf("expected user04999, got %s", results[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_TieBreakingByID(t *testing.T) {
	t.Parallel()
	const docCount = 100
	mockPullerSvc := newMockPuller(docCount + 100)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert documents with same timestamp (tests tie-breaking by ID)
	for i := 0; i < docCount; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(1000), // All same timestamp
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(200 * time.Millisecond)

	// Search - all should be returned, order determined by doc ID as tie-breaker
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      docCount,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != docCount {
		t.Fatalf("expected %d results, got %d", docCount, len(results))
	}

	// All document IDs should be unique
	docIDs := make(map[string]bool)
	for _, ref := range results {
		if docIDs[ref.ID] {
			t.Errorf("duplicate doc ID found: %s", ref.ID)
		}
		docIDs[ref.ID] = true
	}

	// Results should be sorted by ID when timestamps are equal (btree uses ID as tie-breaker)
	ids := make([]string, len(results))
	for i, ref := range results {
		ids[i] = ref.ID
	}
	sortedIDs := make([]string, len(ids))
	copy(sortedIDs, ids)
	sort.Strings(sortedIDs)

	for i := range ids {
		if ids[i] != sortedIDs[i] {
			t.Errorf("result[%d]: expected %s, got %s (tie-breaker should sort by ID)", i, sortedIDs[i], ids[i])
		}
	}

	stopService(t, svc, mockPullerSvc)
}

// ============================================================================
// Dynamic Template Update Tests
// ============================================================================

func TestIntegration_DynamicTemplateReload(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(200)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert documents that match the initial "users" template
	for i := 0; i < 50; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
			"score":     int64(i * 10),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Verify initial template works (ordered by timestamp desc)
	plan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      10,
	}

	results, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("initial search failed: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	if results[0].ID != "user049" {
		t.Errorf("expected user049 first, got %s", results[0].ID)
	}

	// Now dynamically add a new template for ordering by score
	newTemplateYAML := `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: users_by_score
    collectionPattern: "users"
    fields:
      - field: score
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc
`

	// Reload templates via manager
	mgr := svc.Manager()
	err = mgr.LoadTemplatesFromBytes([]byte(newTemplateYAML))
	if err != nil {
		t.Fatalf("failed to reload templates: %v", err)
	}

	// Insert more documents to populate the new index
	for i := 50; i < 100; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
			"score":     int64(i * 10),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("p%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Search using the new score-based template
	scorePlan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "score", Direction: indexer.Desc}},
		Limit:      10,
	}

	scoreResults, err := svc.Search(ctx, "db1", scorePlan)
	if err != nil {
		t.Fatalf("score-based search failed: %v", err)
	}
	if len(scoreResults) != 10 {
		t.Fatalf("expected 10 results for score search, got %d", len(scoreResults))
	}
	// user099 has highest score (990)
	if scoreResults[0].ID != "user099" {
		t.Errorf("expected user099 first for score search, got %s", scoreResults[0].ID)
	}

	// Original timestamp-based search should still work
	timestampResults, err := svc.Search(ctx, "db1", plan)
	if err != nil {
		t.Fatalf("timestamp search after reload failed: %v", err)
	}
	if len(timestampResults) != 10 {
		t.Fatalf("expected 10 results for timestamp search, got %d", len(timestampResults))
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_TemplateAddNewCollection(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(200)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Initially, we have templates for users, orders, products, and chats
	// Let's add events for a collection that doesn't have a template yet

	// Insert events for "comments" collection - no template exists
	for i := 0; i < 20; i++ {
		evt := createTestEvent("db1", "comments", fmt.Sprintf("comment%03d", i), map[string]any{
			"text":      fmt.Sprintf("Comment %d", i),
			"createdAt": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("comment-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Search comments - should fail with no matching index
	commentPlan := indexer.Plan{
		Collection: "comments",
		OrderBy:    []indexer.OrderField{{Field: "createdAt", Direction: indexer.Desc}},
		Limit:      10,
	}

	_, err := svc.Search(ctx, "db1", commentPlan)
	if err != indexer.ErrNoMatchingIndex {
		t.Errorf("expected ErrNoMatchingIndex for comments, got %v", err)
	}

	// Now add a template for comments
	newTemplateYAML := `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc

  - name: comments_by_created
    collectionPattern: "comments"
    fields:
      - field: createdAt
        order: desc
`

	mgr := svc.Manager()
	err = mgr.LoadTemplatesFromBytes([]byte(newTemplateYAML))
	if err != nil {
		t.Fatalf("failed to reload templates with comments: %v", err)
	}

	// Insert more comments - these will be indexed with the new template
	for i := 20; i < 40; i++ {
		evt := createTestEvent("db1", "comments", fmt.Sprintf("comment%03d", i), map[string]any{
			"text":      fmt.Sprintf("Comment %d", i),
			"createdAt": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("comment-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Now search should work for new comments
	results, err := svc.Search(ctx, "db1", commentPlan)
	if err != nil {
		t.Fatalf("search for comments failed after template add: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	// comment039 has highest createdAt
	if results[0].ID != "comment039" {
		t.Errorf("expected comment039 first, got %s", results[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_TemplateModifyFields(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(200)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert products with price and rating
	for i := 0; i < 50; i++ {
		evt := createTestEvent("db1", "products", fmt.Sprintf("prod%03d", i), map[string]any{
			"name":   fmt.Sprintf("Product %d", i),
			"price":  float64(i * 10),
			"rating": float64(5.0 - float64(i%5)*0.5), // ratings: 5.0, 4.5, 4.0, 3.5, 3.0
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("prod-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Initial template orders by price asc
	pricePlan := indexer.Plan{
		Collection: "products",
		OrderBy:    []indexer.OrderField{{Field: "price", Direction: indexer.Asc}},
		Limit:      10,
	}

	results, err := svc.Search(ctx, "db1", pricePlan)
	if err != nil {
		t.Fatalf("price search failed: %v", err)
	}
	if results[0].ID != "prod000" {
		t.Errorf("expected prod000 (lowest price) first, got %s", results[0].ID)
	}

	// Add a new template for rating-based sorting
	newTemplateYAML := `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc

  - name: products_by_rating
    collectionPattern: "products"
    fields:
      - field: rating
        order: desc
      - field: price
        order: asc
`

	mgr := svc.Manager()
	err = mgr.LoadTemplatesFromBytes([]byte(newTemplateYAML))
	if err != nil {
		t.Fatalf("failed to reload templates: %v", err)
	}

	// Insert more products to populate the new rating-based index
	for i := 50; i < 100; i++ {
		evt := createTestEvent("db1", "products", fmt.Sprintf("prod%03d", i), map[string]any{
			"name":   fmt.Sprintf("Product %d", i),
			"price":  float64(i * 10),
			"rating": float64(5.0 - float64(i%5)*0.5),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("prod-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Search by rating desc, price asc
	ratingPlan := indexer.Plan{
		Collection: "products",
		OrderBy: []indexer.OrderField{
			{Field: "rating", Direction: indexer.Desc},
			{Field: "price", Direction: indexer.Asc},
		},
		Limit: 20,
	}

	ratingResults, err := svc.Search(ctx, "db1", ratingPlan)
	if err != nil {
		t.Fatalf("rating search failed: %v", err)
	}
	if len(ratingResults) != 20 {
		t.Fatalf("expected 20 results, got %d", len(ratingResults))
	}

	// Verify ordering: highest rating (5.0) products should be first
	// Only prod050-prod099 are in the new rating index (prod000-prod049 were indexed before template was added)
	// Products with rating 5.0: i % 5 == 0 -> prod050, prod055, prod060, ...
	// Among those, ordered by price asc -> prod050 has lowest price (500)
	if ratingResults[0].ID != "prod050" {
		t.Errorf("expected prod050 (rating 5.0, price 500) first, got %s", ratingResults[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_TemplateRemoveAndReAdd(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(200)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert users
	for i := 0; i < 30; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("user-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Verify users can be searched
	userPlan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      10,
	}

	results, err := svc.Search(ctx, "db1", userPlan)
	if err != nil {
		t.Fatalf("initial user search failed: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}

	// Remove the users template (load templates without it)
	reducedTemplateYAML := `
templates:
  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc
`

	mgr := svc.Manager()
	err = mgr.LoadTemplatesFromBytes([]byte(reducedTemplateYAML))
	if err != nil {
		t.Fatalf("failed to reload reduced templates: %v", err)
	}

	// Search for users should now fail with no matching index
	_, err = svc.Search(ctx, "db1", userPlan)
	if err != indexer.ErrNoMatchingIndex {
		t.Errorf("expected ErrNoMatchingIndex after template removal, got %v", err)
	}

	// Re-add the users template
	fullTemplateYAML := `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc
`

	err = mgr.LoadTemplatesFromBytes([]byte(fullTemplateYAML))
	if err != nil {
		t.Fatalf("failed to reload full templates: %v", err)
	}

	// Insert more users to repopulate the index
	for i := 30; i < 50; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      fmt.Sprintf("User %d", i),
			"timestamp": int64(i),
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("user-%d", i))
	}

	time.Sleep(100 * time.Millisecond)

	// Search should work again for newly inserted users
	results, err = svc.Search(ctx, "db1", userPlan)
	if err != nil {
		t.Fatalf("search after template re-add failed: %v", err)
	}
	if len(results) != 10 {
		t.Fatalf("expected 10 results after re-add, got %d", len(results))
	}
	// user049 should be first (highest timestamp among newly inserted)
	if results[0].ID != "user049" {
		t.Errorf("expected user049 first after re-add, got %s", results[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_MultipleTemplatesForSameCollection(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(500)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Load templates with multiple indexes for the same collection
	multiTemplateYAML := `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: users_by_name
    collectionPattern: "users"
    fields:
      - field: name
        order: asc

  - name: users_by_level_timestamp
    collectionPattern: "users"
    fields:
      - field: level
        order: desc
      - field: timestamp
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc
`

	mgr := svc.Manager()
	err := mgr.LoadTemplatesFromBytes([]byte(multiTemplateYAML))
	if err != nil {
		t.Fatalf("failed to load multi-templates: %v", err)
	}

	// Insert users with name, timestamp, and level
	names := []string{"Alice", "Bob", "Charlie", "David", "Eve"}
	for i := 0; i < 100; i++ {
		evt := createTestEvent("db1", "users", fmt.Sprintf("user%03d", i), map[string]any{
			"name":      names[i%5],
			"timestamp": int64(i),
			"level":     int64(i % 3), // levels: 0, 1, 2
		})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("user-%d", i))
	}

	time.Sleep(200 * time.Millisecond)

	// Search by timestamp desc
	timestampPlan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      10,
	}

	timestampResults, err := svc.Search(ctx, "db1", timestampPlan)
	if err != nil {
		t.Fatalf("timestamp search failed: %v", err)
	}
	if timestampResults[0].ID != "user099" {
		t.Errorf("timestamp search: expected user099 first, got %s", timestampResults[0].ID)
	}

	// Search by name asc
	namePlan := indexer.Plan{
		Collection: "users",
		OrderBy:    []indexer.OrderField{{Field: "name", Direction: indexer.Asc}},
		Limit:      10,
	}

	nameResults, err := svc.Search(ctx, "db1", namePlan)
	if err != nil {
		t.Fatalf("name search failed: %v", err)
	}
	// All results should start with "Alice" (lexicographically first)
	// Users with name "Alice": i % 5 == 0 -> user000, user005, user010, ...
	if nameResults[0].ID != "user000" {
		t.Errorf("name search: expected user000 first, got %s", nameResults[0].ID)
	}

	// Search by level desc, timestamp desc (compound index)
	levelPlan := indexer.Plan{
		Collection: "users",
		OrderBy: []indexer.OrderField{
			{Field: "level", Direction: indexer.Desc},
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 10,
	}

	levelResults, err := svc.Search(ctx, "db1", levelPlan)
	if err != nil {
		t.Fatalf("level search failed: %v", err)
	}
	// Level 2 users have highest level: i % 3 == 2 -> user002, user005, user008, ...
	// Among level 2 users, user098 has highest timestamp
	if levelResults[0].ID != "user098" {
		t.Errorf("level search: expected user098 first, got %s", levelResults[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}

func TestIntegration_TemplatePatternChange(t *testing.T) {
	t.Parallel()
	mockPullerSvc := newMockPuller(300)
	svc, ctx, cancel := setupIndexerService(t, mockPullerSvc)
	defer cancel()

	// Insert chats for different users
	for u := 0; u < 5; u++ {
		for c := 0; c < 20; c++ {
			evt := createTestEvent("db1",
				fmt.Sprintf("users/user%02d/chats", u),
				fmt.Sprintf("chat-u%02d-c%03d", u, c),
				map[string]any{
					"priority":  int64(c % 5),
					"timestamp": int64(u*1000 + c),
				})
			mockPullerSvc.pushEvent(evt, fmt.Sprintf("chat-%d-%d", u, c))
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Search using the pattern template
	chatPlan := indexer.Plan{
		Collection: "users/user00/chats",
		OrderBy: []indexer.OrderField{
			{Field: "priority", Direction: indexer.Desc},
			{Field: "timestamp", Direction: indexer.Desc},
		},
		Limit: 20,
	}

	results, err := svc.Search(ctx, "db1", chatPlan)
	if err != nil {
		t.Fatalf("chat search failed: %v", err)
	}
	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}

	// Add a more specific pattern for user00's chats
	specificTemplateYAML := `
templates:
  - name: users_by_timestamp
    collectionPattern: "users"
    fields:
      - field: timestamp
        order: desc

  - name: user00_chats_by_timestamp
    collectionPattern: "users/user00/chats"
    fields:
      - field: timestamp
        order: desc

  - name: chats_by_priority
    collectionPattern: "users/{userId}/chats"
    fields:
      - field: priority
        order: desc
      - field: timestamp
        order: desc

  - name: orders_by_amount
    collectionPattern: "orders"
    fields:
      - field: amount
        order: desc

  - name: products_by_price
    collectionPattern: "products"
    fields:
      - field: price
        order: asc
`

	mgr := svc.Manager()
	err = mgr.LoadTemplatesFromBytes([]byte(specificTemplateYAML))
	if err != nil {
		t.Fatalf("failed to reload templates with specific pattern: %v", err)
	}

	// Insert more chats for user00
	for c := 20; c < 30; c++ {
		evt := createTestEvent("db1", "users/user00/chats",
			fmt.Sprintf("chat-u00-c%03d", c),
			map[string]any{
				"priority":  int64(c % 5),
				"timestamp": int64(c),
			})
		mockPullerSvc.pushEvent(evt, fmt.Sprintf("chat-specific-%d", c))
	}

	time.Sleep(100 * time.Millisecond)

	// Search user00's chats with timestamp order - should use the more specific template
	specificPlan := indexer.Plan{
		Collection: "users/user00/chats",
		OrderBy:    []indexer.OrderField{{Field: "timestamp", Direction: indexer.Desc}},
		Limit:      10,
	}

	specificResults, err := svc.Search(ctx, "db1", specificPlan)
	if err != nil {
		t.Fatalf("specific pattern search failed: %v", err)
	}
	if len(specificResults) != 10 {
		t.Fatalf("expected 10 results for specific pattern, got %d", len(specificResults))
	}
	// chat-u00-c029 should be first (highest timestamp among newly inserted)
	if specificResults[0].ID != "chat-u00-c029" {
		t.Errorf("expected chat-u00-c029 first, got %s", specificResults[0].ID)
	}

	stopService(t, svc, mockPullerSvc)
}
