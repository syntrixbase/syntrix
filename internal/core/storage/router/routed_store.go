package router

import (
	"context"
	"time"

	"github.com/syntrixbase/syntrix/internal/core/storage/types"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// RoutedDocumentStore implements DocumentStore by routing operations
type RoutedDocumentStore struct {
	router types.DocumentRouter
}

func NewRoutedDocumentStore(router types.DocumentRouter) types.DocumentStore {
	return &RoutedDocumentStore{router: router}
}

func (s *RoutedDocumentStore) Get(ctx context.Context, database string, path string) (*types.StoredDoc, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.Get(ctx, database, path)
}

func (s *RoutedDocumentStore) GetMany(ctx context.Context, database string, paths []string) ([]*types.StoredDoc, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.GetMany(ctx, database, paths)
}

func (s *RoutedDocumentStore) Create(ctx context.Context, database string, doc types.StoredDoc) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Create(ctx, database, doc)
}

func (s *RoutedDocumentStore) Update(ctx context.Context, database string, path string, data map[string]interface{}, pred model.Filters) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Update(ctx, database, path, data, pred)
}

func (s *RoutedDocumentStore) Patch(ctx context.Context, database string, path string, data map[string]interface{}, pred model.Filters) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Patch(ctx, database, path, data, pred)
}

func (s *RoutedDocumentStore) Delete(ctx context.Context, database string, path string, pred model.Filters) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Delete(ctx, database, path, pred)
}

func (s *RoutedDocumentStore) Query(ctx context.Context, database string, q model.Query) ([]*types.StoredDoc, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.Query(ctx, database, q)
}

func (s *RoutedDocumentStore) Watch(ctx context.Context, database string, collection string, resumeToken interface{}, opts types.WatchOptions) (<-chan types.Event, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.Watch(ctx, database, collection, resumeToken, opts)
}

func (s *RoutedDocumentStore) Close(ctx context.Context) error {
	// We don't close the underlying store here as it might be shared.
	// The Provider manages lifecycle.
	return nil
}

// RoutedUserStore implements UserStore by routing operations
type RoutedUserStore struct {
	router types.UserRouter
}

func NewRoutedUserStore(router types.UserRouter) types.UserStore {
	return &RoutedUserStore{router: router}
}

func (s *RoutedUserStore) CreateUser(ctx context.Context, database string, user *types.User) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.CreateUser(ctx, database, user)
}

func (s *RoutedUserStore) GetUserByUsername(ctx context.Context, database string, username string) (*types.User, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.GetUserByUsername(ctx, database, username)
}

func (s *RoutedUserStore) GetUserByID(ctx context.Context, database string, id string) (*types.User, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.GetUserByID(ctx, database, id)
}

func (s *RoutedUserStore) ListUsers(ctx context.Context, database string, limit int, offset int) ([]*types.User, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.ListUsers(ctx, database, limit, offset)
}

func (s *RoutedUserStore) UpdateUser(ctx context.Context, database string, user *types.User) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.UpdateUser(ctx, database, user)
}

func (s *RoutedUserStore) UpdateUserLoginStats(ctx context.Context, database string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.UpdateUserLoginStats(ctx, database, id, lastLogin, attempts, lockoutUntil)
}

func (s *RoutedUserStore) EnsureIndexes(ctx context.Context) error {
	// Ensure indexes on all backends? Or just default?
	// Ideally we should iterate all databases/backends.
	// But Select requires database.
	// For now, let's assume we call it for "default" database or we need a way to iterate.
	// The interface `EnsureIndexes(ctx)` doesn't take database.
	// Wait, `UserStore.EnsureIndexes` DOES NOT take database in my previous update?
	// Let's check `types.go`.
	// `EnsureIndexes(ctx context.Context) error`
	// It does NOT take database.
	// This is a problem if we have multiple backends.
	// But `EnsureIndexes` is usually called at startup.
	// If we have multiple backends, we need to ensure indexes on ALL of them.
	// The `RoutedUserStore` should probably iterate over all known backends.
	// But `RoutedUserStore` only has `router`.
	// The `router` might know about backends.
	// But `router.Select` takes database.
	// If I pass "default", it ensures on default backend.
	// If I have dedicated database, I need to ensure on that backend too.
	// This is a limitation of current `EnsureIndexes` signature.
	// However, `EnsureIndexes` is usually called by `main.go` or `factory`.
	// Maybe `factory` should call `EnsureIndexes` on all created stores directly, instead of relying on `RoutedStore` to do it.
	// The `RoutedStore` implementation of `EnsureIndexes` is tricky.
	// For now, I'll just call it on "default" database, or fail.
	// Or I can update `EnsureIndexes` to take `database`? No, indexes are collection-wide (per backend).
	// If I have multiple backends, I need to run it on each backend.
	// I'll leave `EnsureIndexes` as is for now (calling on default database), and note that factory should handle it.
	// Actually, `factory` creates the stores. It can call `EnsureIndexes` on them.
	// `RoutedStore` might not need to implement `EnsureIndexes` or it should be a no-op if factory handles it.
	// But `UserStore` interface has it.
	// Let's just use "default" for now.
	store, err := s.router.Select("default", types.OpWrite)
	if err != nil {
		return err
	}
	return store.EnsureIndexes(ctx)
}

func (s *RoutedUserStore) Close(ctx context.Context) error {
	return nil
}

// RoutedRevocationStore implements TokenRevocationStore by routing operations
type RoutedRevocationStore struct {
	router types.RevocationRouter
}

func NewRoutedRevocationStore(router types.RevocationRouter) types.TokenRevocationStore {
	return &RoutedRevocationStore{router: router}
}

func (s *RoutedRevocationStore) RevokeToken(ctx context.Context, database string, jti string, expiresAt time.Time) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.RevokeToken(ctx, database, jti, expiresAt)
}

func (s *RoutedRevocationStore) RevokeTokenImmediate(ctx context.Context, database string, jti string, expiresAt time.Time) error {
	store, err := s.router.Select(database, types.OpWrite)
	if err != nil {
		return err
	}
	return store.RevokeTokenImmediate(ctx, database, jti, expiresAt)
}

func (s *RoutedRevocationStore) IsRevoked(ctx context.Context, database string, jti string, gracePeriod time.Duration) (bool, error) {
	store, err := s.router.Select(database, types.OpRead)
	if err != nil {
		return false, err
	}
	return store.IsRevoked(ctx, database, jti, gracePeriod)
}

func (s *RoutedRevocationStore) EnsureIndexes(ctx context.Context) error {
	store, err := s.router.Select("default", types.OpWrite)
	if err != nil {
		return err
	}
	return store.EnsureIndexes(ctx)
}

func (s *RoutedRevocationStore) Close(ctx context.Context) error {
	return nil
}
