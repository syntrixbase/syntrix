package rebuild

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/syntrixbase/syntrix/internal/indexer/internal/index"
	"github.com/syntrixbase/syntrix/internal/indexer/internal/template"
	"github.com/syntrixbase/syntrix/internal/storage/types"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// mockKeyBuilder implements OrderKeyBuilder for testing.
type mockKeyBuilder struct {
	err error
}

func (m *mockKeyBuilder) BuildOrderKey(data map[string]any, tmpl *template.Template) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Simple key based on "id" field
	if id, ok := data["id"].(string); ok {
		return []byte(id), nil
	}
	if id, ok := data["id"].(float64); ok {
		return []byte{byte(id)}, nil
	}
	return []byte{0x00}, nil
}

// mockStorageScanner implements StorageScanner for testing.
type mockStorageScanner struct {
	docs []*types.StoredDoc
	err  error
}

func (m *mockStorageScanner) ScanCollection(ctx context.Context, database, collectionPattern string, batchSize int, startAfter string) (DocIterator, error) {
	if m.err != nil {
		return nil, m.err
	}

	// Find starting position
	startIdx := 0
	if startAfter != "" {
		for i, doc := range m.docs {
			if doc.Id == startAfter {
				startIdx = i + 1
				break
			}
		}
	}

	// Return batch
	endIdx := startIdx + batchSize
	if endIdx > len(m.docs) {
		endIdx = len(m.docs)
	}

	return &mockDocIterator{
		docs: m.docs[startIdx:endIdx],
		idx:  -1,
	}, nil
}

// mockDocIterator implements DocIterator for testing.
type mockDocIterator struct {
	docs []*types.StoredDoc
	idx  int
	err  error
}

func (m *mockDocIterator) Next() bool {
	m.idx++
	return m.idx < len(m.docs)
}

func (m *mockDocIterator) Doc() *types.StoredDoc {
	if m.idx >= 0 && m.idx < len(m.docs) {
		return m.docs[m.idx]
	}
	return nil
}

func (m *mockDocIterator) Err() error {
	return m.err
}

func (m *mockDocIterator) Close() error {
	return nil
}

// mockEventReplayer implements EventReplayer for testing.
type mockEventReplayer struct {
	events []*Event
	err    error
}

func (m *mockEventReplayer) ReplayFrom(ctx context.Context, startKey string, callback func(evt *Event) error) error {
	if m.err != nil {
		return m.err
	}
	for _, evt := range m.events {
		if err := callback(evt); err != nil {
			return err
		}
	}
	return nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 500, cfg.BatchSize)
	assert.Equal(t, 5000, cfg.QPSLimit)
	assert.Equal(t, 2, cfg.MaxConcurrent)
}

func TestNew(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 1000, MaxConcurrent: 1}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	require.NotNil(t, o)
	assert.Equal(t, 100, o.cfg.BatchSize)
	assert.Equal(t, 1000, o.cfg.QPSLimit)
	assert.Equal(t, 1, o.cfg.MaxConcurrent)
}

func TestNew_DefaultValues(t *testing.T) {
	cfg := Config{} // All zeros
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	assert.Equal(t, 500, o.cfg.BatchSize)
	assert.Equal(t, 5000, o.cfg.QPSLimit)
	assert.Equal(t, 2, o.cfg.MaxConcurrent)
}

func TestOrchestrator_StartRebuild(t *testing.T) {
	cfg := Config{BatchSize: 10, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("users/*/chats", "test_template", "users/{uid}/chats")
	tmpl := &template.Template{
		Name:              "test_template",
		CollectionPattern: "users/{uid}/chats",
	}

	scanner := &mockStorageScanner{
		docs: []*types.StoredDoc{
			{Id: "doc1", Data: map[string]any{"id": "a"}},
			{Id: "doc2", Data: map[string]any{"id": "b"}},
			{Id: "doc3", Data: map[string]any{"id": "c"}},
		},
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, jobID)

	// Wait for job to complete
	time.Sleep(200 * time.Millisecond)

	// Check job status
	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, progress.Status)
	assert.Equal(t, int64(3), progress.DocsTotal)
	assert.Equal(t, int64(3), progress.DocsAdded)

	// Index should be healthy
	assert.Equal(t, index.StateHealthy, s.State())
	assert.Equal(t, 3, s.Len())
}

func TestOrchestrator_StartRebuild_AlreadyRunning(t *testing.T) {
	cfg := Config{BatchSize: 1, QPSLimit: 1, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("users/*/chats", "test_template", "users/{uid}/chats")
	tmpl := &template.Template{
		Name:              "test_template",
		CollectionPattern: "users/{uid}/chats",
	}

	// Scanner with many docs to keep job running
	docs := make([]*types.StoredDoc, 100)
	for i := range docs {
		docs[i] = &types.StoredDoc{Id: string(rune('a' + i)), Data: map[string]any{"id": float64(i)}}
	}
	scanner := &mockStorageScanner{docs: docs}

	ctx := context.Background()
	jobID1, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)

	// Try to start another rebuild for same index
	_, err = o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already in progress")

	// Cancel the first job
	err = o.CancelRebuild(jobID1)
	require.NoError(t, err)

	// Wait for cancellation
	time.Sleep(100 * time.Millisecond)
}

func TestOrchestrator_CancelRebuild(t *testing.T) {
	cfg := Config{BatchSize: 1, QPSLimit: 1, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}

	// Scanner with many docs
	docs := make([]*types.StoredDoc, 1000)
	for i := range docs {
		docs[i] = &types.StoredDoc{Id: string(rune(i)), Data: map[string]any{"id": float64(i)}}
	}
	scanner := &mockStorageScanner{docs: docs}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)

	// Wait for job to start
	time.Sleep(50 * time.Millisecond)

	// Cancel
	err = o.CancelRebuild(jobID)
	require.NoError(t, err)

	// Wait for cancellation to complete
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusCanceled, progress.Status)
}

func TestOrchestrator_ListJobs(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s1 := index.New("a", "id", "a")
	s2 := index.New("b", "id", "b")
	tmpl := &template.Template{Name: "test"}
	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}

	ctx := context.Background()
	_, err := o.StartRebuild(ctx, s1, tmpl, "db1", scanner, nil)
	require.NoError(t, err)
	_, err = o.StartRebuild(ctx, s2, tmpl, "db2", scanner, nil)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	jobs := o.ListJobs()
	assert.Len(t, jobs, 2)
}

func TestOrchestrator_GetJob_NotFound(t *testing.T) {
	o := New(DefaultConfig(), &mockKeyBuilder{}, testLogger())

	_, err := o.GetJob("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOrchestrator_CancelRebuild_NotFound(t *testing.T) {
	o := New(DefaultConfig(), &mockKeyBuilder{}, testLogger())

	err := o.CancelRebuild("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOrchestrator_CleanupCompletedJobs(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}
	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}

	ctx := context.Background()
	_, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	// Should not remove (too recent)
	removed := o.CleanupCompletedJobs(time.Hour)
	assert.Equal(t, 0, removed)

	// Should remove (max age 0)
	removed = o.CleanupCompletedJobs(0)
	assert.Equal(t, 1, removed)

	// Job list should be empty
	jobs := o.ListJobs()
	assert.Len(t, jobs, 0)
}

func TestOrchestrator_WithEventReplay(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}

	scanner := &mockStorageScanner{
		docs: []*types.StoredDoc{
			{Id: "doc1", Data: map[string]any{"id": "a"}},
		},
	}

	replayer := &mockEventReplayer{
		events: []*Event{
			{DatabaseID: "testdb", DocID: "doc2", Data: map[string]any{"id": "b"}},
			{DatabaseID: "testdb", DocID: "doc3", Data: map[string]any{"id": "c"}},
		},
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, replayer)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, progress.Status)

	// Should have 3 docs (1 from storage + 2 from replay)
	assert.Equal(t, 3, s.Len())
}

func TestOrchestrator_ScannerError(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}

	scanner := &mockStorageScanner{
		err: errors.New("storage error"),
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, progress.Status)
	assert.Contains(t, progress.Error, "storage error")
	assert.Equal(t, index.StateFailed, s.State())
}

func TestOrchestrator_MaxConcurrent(t *testing.T) {
	cfg := Config{BatchSize: 1, QPSLimit: 1, MaxConcurrent: 1}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	// Create slow scanners
	docs := make([]*types.StoredDoc, 100)
	for i := range docs {
		docs[i] = &types.StoredDoc{Id: string(rune(i)), Data: map[string]any{"id": float64(i)}}
	}
	scanner := &mockStorageScanner{docs: docs}

	s1 := index.New("a", "id", "a")
	s2 := index.New("b", "id", "b")
	tmpl := &template.Template{Name: "test"}

	ctx := context.Background()

	// Start first job
	jobID1, err := o.StartRebuild(ctx, s1, tmpl, "db1", scanner, nil)
	require.NoError(t, err)

	// Wait for it to start
	time.Sleep(50 * time.Millisecond)

	// Start second job (should be queued)
	jobID2, err := o.StartRebuild(ctx, s2, tmpl, "db2", scanner, nil)
	require.NoError(t, err)

	// Second job should be pending
	progress2, err := o.GetJob(jobID2)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, progress2.Status)

	// Cancel first job to allow second to run
	err = o.CancelRebuild(jobID1)
	require.NoError(t, err)

	// Wait and cleanup
	time.Sleep(100 * time.Millisecond)
	o.CancelRebuild(jobID2)
}

func TestJob_Progress(t *testing.T) {
	job := &Job{
		ID:        "test-job",
		Status:    StatusRunning,
		DocsTotal: 100,
		DocsAdded: 50,
		StartTime: time.Now().Add(-time.Minute),
	}

	progress := job.Progress()
	assert.Equal(t, "test-job", progress.ID)
	assert.Equal(t, StatusRunning, progress.Status)
	assert.Equal(t, int64(100), progress.DocsTotal)
	assert.Equal(t, int64(50), progress.DocsAdded)
}

func TestOrchestrator_DeletedEvents(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}

	scanner := &mockStorageScanner{
		docs: []*types.StoredDoc{
			{Id: "doc1", Data: map[string]any{"id": "a"}},
			{Id: "doc2", Data: map[string]any{"id": "b"}},
		},
	}

	replayer := &mockEventReplayer{
		events: []*Event{
			{DatabaseID: "testdb", DocID: "doc1", Deleted: true}, // Delete doc1
		},
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, replayer)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, progress.Status)

	// Should have 1 doc (doc2 only, doc1 was deleted)
	assert.Equal(t, 1, s.Len())
}

func TestOrchestrator_ConcurrentAccess(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 10}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}
	tmpl := &template.Template{Name: "test"}

	var wg sync.WaitGroup
	ctx := context.Background()

	// Start multiple rebuilds concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s := index.New(string(rune('a'+idx)), "id", string(rune('a'+idx)))
			_, err := o.StartRebuild(ctx, s, tmpl, "db", scanner, nil)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Wait for all to complete
	time.Sleep(200 * time.Millisecond)

	jobs := o.ListJobs()
	assert.Len(t, jobs, 10)

	for _, j := range jobs {
		assert.Equal(t, StatusCompleted, j.Status)
	}
}

func TestOrchestrator_CancelRunningJob(t *testing.T) {
	cfg := Config{BatchSize: 1, QPSLimit: 1, MaxConcurrent: 1}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	// Create a slow scanner with many docs
	docs := make([]*types.StoredDoc, 1000)
	for i := range docs {
		docs[i] = &types.StoredDoc{Id: string(rune(i)), Data: map[string]any{"id": float64(i)}}
	}
	scanner := &mockStorageScanner{docs: docs}

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)

	// Wait for it to start running
	time.Sleep(50 * time.Millisecond)

	// Cancel the running job
	err = o.CancelRebuild(jobID)
	require.NoError(t, err)

	// Wait for cancellation to take effect
	time.Sleep(100 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	// Should be canceled or failed (due to context cancellation)
	assert.True(t, progress.Status == StatusCanceled || progress.Status == StatusFailed)
}

func TestOrchestrator_CancelCompletedJob(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}
	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, nil)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	// Try to cancel a completed job - should fail
	err = o.CancelRebuild(jobID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not pending or running")
}

func TestOrchestrator_ReplayEventsError(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}
	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}

	// Create a replayer that returns an error
	replayer := &mockEventReplayer{
		err: errors.New("replay error"),
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, replayer)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, progress.Status)
	assert.Contains(t, progress.Error, "replay error")
}

func TestOrchestrator_ReplayDifferentDatabase(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	o := New(cfg, &mockKeyBuilder{}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}
	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}

	// Events from a different database should be ignored
	replayer := &mockEventReplayer{
		events: []*Event{
			{DatabaseID: "other-db", DocID: "doc1", Data: map[string]any{"id": "a"}},
		},
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, replayer)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, progress.Status)

	// Should have 0 docs (event was from different database)
	assert.Equal(t, 0, s.Len())
}

func TestOrchestrator_ReplayBuildKeyError(t *testing.T) {
	cfg := Config{BatchSize: 100, QPSLimit: 10000, MaxConcurrent: 2}
	// Use a key builder that fails
	o := New(cfg, &mockKeyBuilder{err: errors.New("build key error")}, testLogger())

	s := index.New("test", "id", "test")
	tmpl := &template.Template{Name: "test"}
	scanner := &mockStorageScanner{docs: []*types.StoredDoc{}}

	replayer := &mockEventReplayer{
		events: []*Event{
			{DatabaseID: "testdb", DocID: "doc1", Data: map[string]any{"id": "a"}},
		},
	}

	ctx := context.Background()
	jobID, err := o.StartRebuild(ctx, s, tmpl, "testdb", scanner, replayer)
	require.NoError(t, err)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	progress, err := o.GetJob(jobID)
	require.NoError(t, err)
	// Should complete (error is logged but not fatal)
	assert.Equal(t, StatusCompleted, progress.Status)

	// Doc should not be added due to key build error
	assert.Equal(t, 0, s.Len())
}
