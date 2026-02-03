package database

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/core/storage/types"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// mockDBStore implements DatabaseStore for testing
type mockDBStore struct {
	listFunc   func(ctx context.Context, opts ListOptions) ([]*Database, int, error)
	deleteFunc func(ctx context.Context, id string) error
	databases  []*Database
}

func (m *mockDBStore) Create(ctx context.Context, db *Database) error {
	m.databases = append(m.databases, db)
	return nil
}

func (m *mockDBStore) Get(ctx context.Context, id string) (*Database, error) {
	for _, db := range m.databases {
		if db.ID == id {
			return db, nil
		}
	}
	return nil, ErrDatabaseNotFound
}

func (m *mockDBStore) GetBySlug(ctx context.Context, slug string) (*Database, error) {
	return nil, ErrDatabaseNotFound
}

func (m *mockDBStore) List(ctx context.Context, opts ListOptions) ([]*Database, int, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, opts)
	}
	return m.databases, len(m.databases), nil
}

func (m *mockDBStore) Update(ctx context.Context, db *Database) error {
	return nil
}

func (m *mockDBStore) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	// Remove from databases
	for i, db := range m.databases {
		if db.ID == id {
			m.databases = append(m.databases[:i], m.databases[i+1:]...)
			return nil
		}
	}
	return ErrDatabaseNotFound
}

func (m *mockDBStore) CountByOwner(ctx context.Context, ownerID string) (int, error) {
	return 0, nil
}

func (m *mockDBStore) Exists(ctx context.Context, id string) (bool, error) {
	for _, db := range m.databases {
		if db.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockDBStore) Close(ctx context.Context) error { return nil }

// mockIndexInvalidator implements IndexInvalidator for testing
type mockIndexInvalidator struct {
	invalidateFunc func(ctx context.Context, database string) error
	callCount      atomic.Int32
}

func (m *mockIndexInvalidator) InvalidateDatabase(ctx context.Context, database string) error {
	m.callCount.Add(1)
	if m.invalidateFunc != nil {
		return m.invalidateFunc(ctx, database)
	}
	return nil
}

// fullMockDocStore implements the full types.DocumentStore interface
type fullMockDocStore struct {
	deleteByDBFunc func(ctx context.Context, database string, limit int) (int, error)
}

func (m *fullMockDocStore) Get(ctx context.Context, database string, path string) (*types.StoredDoc, error) {
	return nil, nil
}
func (m *fullMockDocStore) GetMany(ctx context.Context, database string, paths []string) ([]*types.StoredDoc, error) {
	return nil, nil
}
func (m *fullMockDocStore) Create(ctx context.Context, database string, doc types.StoredDoc) error {
	return nil
}
func (m *fullMockDocStore) Update(ctx context.Context, database string, path string, data map[string]interface{}, pred model.Filters) error {
	return nil
}
func (m *fullMockDocStore) Patch(ctx context.Context, database string, path string, data map[string]interface{}, pred model.Filters) error {
	return nil
}
func (m *fullMockDocStore) Delete(ctx context.Context, database string, path string, pred model.Filters) error {
	return nil
}
func (m *fullMockDocStore) DeleteByDatabase(ctx context.Context, database string, limit int) (int, error) {
	if m.deleteByDBFunc != nil {
		return m.deleteByDBFunc(ctx, database, limit)
	}
	return 0, nil
}
func (m *fullMockDocStore) Query(ctx context.Context, database string, q model.Query) ([]*types.StoredDoc, error) {
	return nil, nil
}
func (m *fullMockDocStore) Watch(ctx context.Context, database string, collection string, resumeToken interface{}, opts types.WatchOptions) (<-chan types.Event, error) {
	return nil, nil
}
func (m *fullMockDocStore) Close(ctx context.Context) error { return nil }

func TestDeletionWorker_NewDeletionWorker(t *testing.T) {
	dbStore := &mockDBStore{}
	docStore := &fullMockDocStore{}
	logger := slog.Default()

	t.Run("with valid config", func(t *testing.T) {
		config := DeletionWorkerConfig{
			Interval:           1 * time.Minute,
			BatchSize:          50,
			MaxBatchesPerCycle: 5,
		}
		worker := NewDeletionWorker(dbStore, docStore, nil, config, logger)

		assert.NotNil(t, worker)
		assert.Equal(t, 50, worker.config.BatchSize)
		assert.Equal(t, 5, worker.config.MaxBatchesPerCycle)
	})

	t.Run("with zero config uses defaults", func(t *testing.T) {
		config := DeletionWorkerConfig{} // all zeros
		worker := NewDeletionWorker(dbStore, docStore, nil, config, logger)

		assert.NotNil(t, worker)
		// Defaults are applied
		assert.Equal(t, 100, worker.config.BatchSize)
		assert.Equal(t, 10, worker.config.MaxBatchesPerCycle)
	})
}

func TestDeletionWorker_StartStop(t *testing.T) {
	dbStore := &mockDBStore{
		listFunc: func(ctx context.Context, opts ListOptions) ([]*Database, int, error) {
			return []*Database{}, 0, nil // No databases to delete
		},
	}
	docStore := &fullMockDocStore{}
	logger := slog.Default()

	config := DeletionWorkerConfig{
		Interval:           50 * time.Millisecond,
		BatchSize:          10,
		MaxBatchesPerCycle: 1,
	}

	worker := NewDeletionWorker(dbStore, docStore, nil, config, logger)

	t.Run("start and stop", func(t *testing.T) {
		ctx := context.Background()

		// Start
		err := worker.Start(ctx)
		require.NoError(t, err)
		assert.True(t, worker.IsRunning())

		// Wait a bit
		time.Sleep(100 * time.Millisecond)

		// Stop
		err = worker.Stop(context.Background())
		require.NoError(t, err)
		assert.False(t, worker.IsRunning())
	})

	t.Run("double start is no-op", func(t *testing.T) {
		ctx := context.Background()

		err := worker.Start(ctx)
		require.NoError(t, err)

		err = worker.Start(ctx) // Second start should be no-op
		require.NoError(t, err)

		err = worker.Stop(context.Background())
		require.NoError(t, err)
	})
}

func TestDeletionWorker_ProcessDatabases(t *testing.T) {
	slug := "test-db"
	deletingDB := &Database{
		ID:     "db123",
		Slug:   &slug,
		Status: StatusDeleting,
	}

	deletedDBs := make([]string, 0)
	invalidatedDBs := make([]string, 0)

	dbStore := &mockDBStore{
		databases: []*Database{deletingDB},
		listFunc: func(ctx context.Context, opts ListOptions) ([]*Database, int, error) {
			if opts.Status == StatusDeleting {
				result := make([]*Database, 0)
				for _, db := range []*Database{deletingDB} {
					if db.Status == StatusDeleting {
						result = append(result, db)
					}
				}
				return result, len(result), nil
			}
			return []*Database{}, 0, nil
		},
		deleteFunc: func(ctx context.Context, id string) error {
			deletedDBs = append(deletedDBs, id)
			return nil
		},
	}

	docDeleteCalls := 0
	docStore := &fullMockDocStore{
		deleteByDBFunc: func(ctx context.Context, database string, limit int) (int, error) {
			docDeleteCalls++
			if docDeleteCalls == 1 {
				return 0, nil // No documents to delete
			}
			return 0, nil
		},
	}

	indexer := &mockIndexInvalidator{
		invalidateFunc: func(ctx context.Context, database string) error {
			invalidatedDBs = append(invalidatedDBs, database)
			return nil
		},
	}

	logger := slog.Default()
	config := DeletionWorkerConfig{
		Interval:           50 * time.Millisecond,
		BatchSize:          10,
		MaxBatchesPerCycle: 1,
	}

	worker := NewDeletionWorker(dbStore, docStore, indexer, config, logger)

	// Run one cycle manually
	ctx := context.Background()
	worker.processDeletingDatabases(ctx)

	// Verify database was processed
	assert.Contains(t, deletedDBs, "db123")
	assert.Contains(t, invalidatedDBs, "db123")
}

func TestDeletionWorker_BatchDeletion(t *testing.T) {
	slug := "batch-db"
	deletingDB := &Database{
		ID:     "db-batch",
		Slug:   &slug,
		Status: StatusDeleting,
	}

	dbStore := &mockDBStore{
		databases: []*Database{deletingDB},
		listFunc: func(ctx context.Context, opts ListOptions) ([]*Database, int, error) {
			if opts.Status == StatusDeleting {
				return []*Database{deletingDB}, 1, nil
			}
			return []*Database{}, 0, nil
		},
	}

	batchCalls := 0
	docStore := &fullMockDocStore{
		deleteByDBFunc: func(ctx context.Context, database string, limit int) (int, error) {
			batchCalls++
			// First batch returns full batch size (more to delete)
			// Second batch returns 0 (no more documents)
			if batchCalls <= 2 {
				return limit, nil // Return full batch
			}
			return 0, nil // No more documents
		},
	}

	logger := slog.Default()
	config := DeletionWorkerConfig{
		Interval:           50 * time.Millisecond,
		BatchSize:          10,
		MaxBatchesPerCycle: 3, // Allow 3 batches per cycle
	}

	worker := NewDeletionWorker(dbStore, docStore, nil, config, logger)

	// Run one cycle
	ctx := context.Background()
	worker.processDeletingDatabases(ctx)

	// Should have called delete at least 3 times (batches) + 1 (check for remaining)
	assert.GreaterOrEqual(t, batchCalls, 3)
}

func TestDeletionWorker_LastRunTime(t *testing.T) {
	dbStore := &mockDBStore{
		listFunc: func(ctx context.Context, opts ListOptions) ([]*Database, int, error) {
			return []*Database{}, 0, nil
		},
	}
	docStore := &fullMockDocStore{}
	logger := slog.Default()

	config := DeletionWorkerConfig{
		Interval:           50 * time.Millisecond,
		BatchSize:          10,
		MaxBatchesPerCycle: 1,
	}

	worker := NewDeletionWorker(dbStore, docStore, nil, config, logger)

	// Before running, last run time should be zero
	assert.True(t, worker.LastRunTime().IsZero())

	// Run one cycle
	ctx := context.Background()
	worker.processDeletingDatabases(ctx)

	// After running, last run time should be set
	assert.False(t, worker.LastRunTime().IsZero())
	assert.WithinDuration(t, time.Now(), worker.LastRunTime(), 1*time.Second)
}
