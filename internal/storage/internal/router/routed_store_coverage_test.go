package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/syntrixbase/syntrix/internal/storage/types"
	"github.com/syntrixbase/syntrix/pkg/model"
)

func TestRoutedDocumentStore_Coverage(t *testing.T) {
	ctx := context.Background()
	database := "default"
	errSelect := errors.New("select error")

	t.Run("Get Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Get(ctx, database, "path")

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Create Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Create(ctx, database, types.StoredDoc{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Update Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Update(ctx, database, "path", nil, nil)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Patch Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Patch(ctx, database, "path", nil, nil)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Delete Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Delete(ctx, database, "path", nil)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Query Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Query(ctx, database, model.Query{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Watch Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Watch(ctx, database, "coll", nil, types.WatchOptions{})

		assert.ErrorIs(t, err, errSelect)
	})
}

func TestRoutedUserStore_Coverage(t *testing.T) {
	ctx := context.Background()
	database := "default"
	errSelect := errors.New("select error")

	t.Run("CreateUser Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.CreateUser(ctx, database, &types.User{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("GetUserByUsername Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		_, err := rs.GetUserByUsername(ctx, database, "user")

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("GetUserByID Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		_, err := rs.GetUserByID(ctx, database, "id")

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("ListUsers Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		_, err := rs.ListUsers(ctx, database, 10, 0)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("UpdateUser Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.UpdateUser(ctx, database, &types.User{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("UpdateUserLoginStats Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.UpdateUserLoginStats(ctx, database, "id", time.Now(), 0, time.Time{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("EnsureIndexes Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", "default", types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.EnsureIndexes(ctx)

		assert.ErrorIs(t, err, errSelect)
	})
}

func TestRoutedRevocationStore_Coverage(t *testing.T) {
	ctx := context.Background()
	database := "default"
	errSelect := errors.New("select error")

	t.Run("RevokeToken Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		err := rs.RevokeToken(ctx, database, "jti", time.Now())

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("RevokeTokenImmediate Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", database, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		err := rs.RevokeTokenImmediate(ctx, database, "jti", time.Now())

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("IsRevoked Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", database, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		_, err := rs.IsRevoked(ctx, database, "jti", time.Minute)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("EnsureIndexes Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", "default", types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		err := rs.EnsureIndexes(ctx)

		assert.ErrorIs(t, err, errSelect)
	})
}
