package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codetrek/syntrix/internal/storage/types"
	"github.com/codetrek/syntrix/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestRoutedDocumentStore_Coverage(t *testing.T) {
	ctx := context.Background()
	tenant := "default"
	errSelect := errors.New("select error")

	t.Run("Get Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Get(ctx, tenant, "path")

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Create Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Create(ctx, tenant, &types.Document{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Update Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Update(ctx, tenant, "path", nil, nil)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Patch Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Patch(ctx, tenant, "path", nil, nil)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Delete Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		err := rs.Delete(ctx, tenant, "path", nil)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Query Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Query(ctx, tenant, model.Query{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("Watch Select Error", func(t *testing.T) {
		router := new(mockDocRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Watch(ctx, tenant, "coll", nil, types.WatchOptions{})

		assert.ErrorIs(t, err, errSelect)
	})
}

func TestRoutedUserStore_Coverage(t *testing.T) {
	ctx := context.Background()
	tenant := "default"
	errSelect := errors.New("select error")

	t.Run("CreateUser Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.CreateUser(ctx, tenant, &types.User{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("GetUserByUsername Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		_, err := rs.GetUserByUsername(ctx, tenant, "user")

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("GetUserByID Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		_, err := rs.GetUserByID(ctx, tenant, "id")

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("ListUsers Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		_, err := rs.ListUsers(ctx, tenant, 10, 0)

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("UpdateUser Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.UpdateUser(ctx, tenant, &types.User{})

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("UpdateUserLoginStats Select Error", func(t *testing.T) {
		router := new(mockUserRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedUserStore(router)
		err := rs.UpdateUserLoginStats(ctx, tenant, "id", time.Now(), 0, time.Time{})

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
	tenant := "default"
	errSelect := errors.New("select error")

	t.Run("RevokeToken Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		err := rs.RevokeToken(ctx, tenant, "jti", time.Now())

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("RevokeTokenImmediate Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", tenant, types.OpWrite).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		err := rs.RevokeTokenImmediate(ctx, tenant, "jti", time.Now())

		assert.ErrorIs(t, err, errSelect)
	})

	t.Run("IsRevoked Select Error", func(t *testing.T) {
		router := new(mockRevRouter)
		router.On("Select", tenant, types.OpRead).Return(nil, errSelect)

		rs := NewRoutedRevocationStore(router)
		_, err := rs.IsRevoked(ctx, tenant, "jti", time.Minute)

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
