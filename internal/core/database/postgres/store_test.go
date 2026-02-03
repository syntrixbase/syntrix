package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/syntrixbase/syntrix/internal/core/database"
)

func TestStore_Create(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	slug := "my-app"
	desc := "Test database"
	testDB := &database.Database{
		ID:          "a1b2c3d4e5f67890",
		Slug:        &slug,
		DisplayName: "My App",
		Description: &desc,
		OwnerID:     "user-123",
		Status:      database.StatusActive,
	}

	mock.ExpectExec(`INSERT INTO databases`).
		WithArgs(
			testDB.ID, testDB.Slug, testDB.DisplayName, testDB.Description, testDB.OwnerID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), int64(0), int64(0), database.StatusActive,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.Create(context.Background(), testDB)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Get(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	slug := "my-app"
	desc := "Test database"
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "slug", "display_name", "description", "owner_id",
		"created_at", "updated_at", "max_documents", "max_storage_bytes", "status",
	}).AddRow(
		"a1b2c3d4e5f67890", slug, "My App", desc, "user-123",
		now, now, int64(0), int64(0), "active",
	)

	mock.ExpectQuery(`SELECT .+ FROM databases WHERE id = \$1`).
		WithArgs("a1b2c3d4e5f67890").
		WillReturnRows(rows)

	result, err := store.Get(context.Background(), "a1b2c3d4e5f67890")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "a1b2c3d4e5f67890", result.ID)
	assert.Equal(t, &slug, result.Slug)
	assert.Equal(t, "My App", result.DisplayName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Get_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	rows := sqlmock.NewRows([]string{
		"id", "slug", "display_name", "description", "owner_id",
		"created_at", "updated_at", "max_documents", "max_storage_bytes", "status",
	})

	mock.ExpectQuery(`SELECT .+ FROM databases WHERE id = \$1`).
		WithArgs("nonexistent").
		WillReturnRows(rows)

	result, err := store.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, database.ErrDatabaseNotFound)
	assert.Nil(t, result)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_GetBySlug(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	slug := "my-app"
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "slug", "display_name", "description", "owner_id",
		"created_at", "updated_at", "max_documents", "max_storage_bytes", "status",
	}).AddRow(
		"a1b2c3d4e5f67890", slug, "My App", nil, "user-123",
		now, now, int64(0), int64(0), "active",
	)

	mock.ExpectQuery(`SELECT .+ FROM databases WHERE slug = \$1`).
		WithArgs("my-app").
		WillReturnRows(rows)

	result, err := store.GetBySlug(context.Background(), "my-app")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "a1b2c3d4e5f67890", result.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_List(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	now := time.Now()

	// Count query
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs(nil, nil).
		WillReturnRows(countRows)

	// List query
	listRows := sqlmock.NewRows([]string{
		"id", "slug", "display_name", "description", "owner_id",
		"created_at", "updated_at", "max_documents", "max_storage_bytes", "status",
	}).AddRow(
		"a1b2c3d4e5f67890", "app-1", "App 1", nil, "user-123",
		now, now, int64(0), int64(0), "active",
	).AddRow(
		"b2c3d4e5f6789012", "app-2", "App 2", nil, "user-123",
		now, now, int64(0), int64(0), "active",
	)

	mock.ExpectQuery(`SELECT .+ FROM databases`).
		WithArgs(nil, nil, 20, 0).
		WillReturnRows(listRows)

	results, total, err := store.List(context.Background(), database.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_List_WithFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	now := time.Now()

	// Count query with owner filter
	countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
	mock.ExpectQuery(`SELECT COUNT\(\*\)`).
		WithArgs("user-123", "active").
		WillReturnRows(countRows)

	// List query with owner filter
	listRows := sqlmock.NewRows([]string{
		"id", "slug", "display_name", "description", "owner_id",
		"created_at", "updated_at", "max_documents", "max_storage_bytes", "status",
	}).AddRow(
		"a1b2c3d4e5f67890", "app-1", "App 1", nil, "user-123",
		now, now, int64(0), int64(0), "active",
	)

	mock.ExpectQuery(`SELECT .+ FROM databases`).
		WithArgs("user-123", "active", 10, 5).
		WillReturnRows(listRows)

	results, total, err := store.List(context.Background(), database.ListOptions{
		OwnerID: "user-123",
		Status:  database.StatusActive,
		Limit:   10,
		Offset:  5,
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Update(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	slug := "my-app"
	desc := "Updated description"
	testDB := &database.Database{
		ID:              "a1b2c3d4e5f67890",
		Slug:            &slug,
		DisplayName:     "My App Updated",
		Description:     &desc,
		OwnerID:         "user-123",
		Status:          database.StatusActive,
		MaxDocuments:    1000,
		MaxStorageBytes: 10485760,
	}

	mock.ExpectExec(`UPDATE databases SET`).
		WithArgs(
			testDB.ID, testDB.Slug, testDB.DisplayName, testDB.Description,
			testDB.Status, testDB.MaxDocuments, testDB.MaxStorageBytes, sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.Update(context.Background(), testDB)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Update_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	testDB := &database.Database{
		ID:          "nonexistent",
		DisplayName: "Test",
		OwnerID:     "user-123",
		Status:      database.StatusActive,
	}

	mock.ExpectExec(`UPDATE databases SET`).
		WithArgs(
			testDB.ID, nil, testDB.DisplayName, nil,
			testDB.Status, int64(0), int64(0), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.Update(context.Background(), testDB)
	assert.ErrorIs(t, err, database.ErrDatabaseNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Delete(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	mock.ExpectExec(`DELETE FROM databases WHERE id = \$1`).
		WithArgs("a1b2c3d4e5f67890").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.Delete(context.Background(), "a1b2c3d4e5f67890")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Delete_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	mock.ExpectExec(`DELETE FROM databases WHERE id = \$1`).
		WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = store.Delete(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, database.ErrDatabaseNotFound)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_CountByOwner(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	rows := sqlmock.NewRows([]string{"count"}).AddRow(3)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM databases WHERE owner_id = \$1`).
		WithArgs("user-123").
		WillReturnRows(rows)

	count, err := store.CountByOwner(context.Background(), "user-123")
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Exists(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("a1b2c3d4e5f67890").
		WillReturnRows(rows)

	exists, err := store.Exists(context.Background(), "a1b2c3d4e5f67890")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestStore_Exists_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := NewStore(db, "databases")

	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs("nonexistent").
		WillReturnRows(rows)

	exists, err := store.Exists(context.Background(), "nonexistent")
	assert.NoError(t, err)
	assert.False(t, exists)
	assert.NoError(t, mock.ExpectationsWereMet())
}
