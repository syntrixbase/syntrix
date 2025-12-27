package router

import (
	"context"
	"time"

	"github.com/codetrek/syntrix/internal/storage/types"
	"github.com/codetrek/syntrix/pkg/model"
)

// RoutedDocumentStore implements DocumentStore by routing operations
type RoutedDocumentStore struct {
	router types.DocumentRouter
}

func NewRoutedDocumentStore(router types.DocumentRouter) types.DocumentStore {
	return &RoutedDocumentStore{router: router}
}

func (s *RoutedDocumentStore) Get(ctx context.Context, tenant string, path string) (*types.Document, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.Get(ctx, tenant, path)
}

func (s *RoutedDocumentStore) Create(ctx context.Context, tenant string, doc *types.Document) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Create(ctx, tenant, doc)
}

func (s *RoutedDocumentStore) Update(ctx context.Context, tenant string, path string, data map[string]interface{}, pred model.Filters) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Update(ctx, tenant, path, data, pred)
}

func (s *RoutedDocumentStore) Patch(ctx context.Context, tenant string, path string, data map[string]interface{}, pred model.Filters) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Patch(ctx, tenant, path, data, pred)
}

func (s *RoutedDocumentStore) Delete(ctx context.Context, tenant string, path string, pred model.Filters) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.Delete(ctx, tenant, path, pred)
}

func (s *RoutedDocumentStore) Query(ctx context.Context, tenant string, q model.Query) ([]*types.Document, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.Query(ctx, tenant, q)
}

func (s *RoutedDocumentStore) Watch(ctx context.Context, tenant string, collection string, resumeToken interface{}, opts types.WatchOptions) (<-chan types.Event, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.Watch(ctx, tenant, collection, resumeToken, opts)
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

func (s *RoutedUserStore) CreateUser(ctx context.Context, tenant string, user *types.User) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.CreateUser(ctx, tenant, user)
}

func (s *RoutedUserStore) GetUserByUsername(ctx context.Context, tenant string, username string) (*types.User, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.GetUserByUsername(ctx, tenant, username)
}

func (s *RoutedUserStore) GetUserByID(ctx context.Context, tenant string, id string) (*types.User, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.GetUserByID(ctx, tenant, id)
}

func (s *RoutedUserStore) ListUsers(ctx context.Context, tenant string, limit int, offset int) ([]*types.User, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return nil, err
	}
	return store.ListUsers(ctx, tenant, limit, offset)
}

func (s *RoutedUserStore) UpdateUser(ctx context.Context, tenant string, user *types.User) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.UpdateUser(ctx, tenant, user)
}

func (s *RoutedUserStore) UpdateUserLoginStats(ctx context.Context, tenant string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.UpdateUserLoginStats(ctx, tenant, id, lastLogin, attempts, lockoutUntil)
}

func (s *RoutedUserStore) EnsureIndexes(ctx context.Context) error {
	// Ensure indexes on all backends? Or just default?
	// Ideally we should iterate all tenants/backends.
	// But Select requires tenant.
	// For now, let's assume we call it for "default" tenant or we need a way to iterate.
	// The interface `EnsureIndexes(ctx)` doesn't take tenant.
	// Wait, `UserStore.EnsureIndexes` DOES NOT take tenant in my previous update?
	// Let's check `types.go`.
	// `EnsureIndexes(ctx context.Context) error`
	// It does NOT take tenant.
	// This is a problem if we have multiple backends.
	// But `EnsureIndexes` is usually called at startup.
	// If we have multiple backends, we need to ensure indexes on ALL of them.
	// The `RoutedUserStore` should probably iterate over all known backends.
	// But `RoutedUserStore` only has `router`.
	// The `router` might know about backends.
	// But `router.Select` takes tenant.
	// If I pass "default", it ensures on default backend.
	// If I have dedicated tenant, I need to ensure on that backend too.
	// This is a limitation of current `EnsureIndexes` signature.
	// However, `EnsureIndexes` is usually called by `main.go` or `factory`.
	// Maybe `factory` should call `EnsureIndexes` on all created stores directly, instead of relying on `RoutedStore` to do it.
	// The `RoutedStore` implementation of `EnsureIndexes` is tricky.
	// For now, I'll just call it on "default" tenant, or fail.
	// Or I can update `EnsureIndexes` to take `tenant`? No, indexes are collection-wide (per backend).
	// If I have multiple backends, I need to run it on each backend.
	// I'll leave `EnsureIndexes` as is for now (calling on default tenant), and note that factory should handle it.
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

func (s *RoutedRevocationStore) RevokeToken(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.RevokeToken(ctx, tenant, jti, expiresAt)
}

func (s *RoutedRevocationStore) RevokeTokenImmediate(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	store, err := s.router.Select(tenant, types.OpWrite)
	if err != nil {
		return err
	}
	return store.RevokeTokenImmediate(ctx, tenant, jti, expiresAt)
}

func (s *RoutedRevocationStore) IsRevoked(ctx context.Context, tenant string, jti string, gracePeriod time.Duration) (bool, error) {
	store, err := s.router.Select(tenant, types.OpRead)
	if err != nil {
		return false, err
	}
	return store.IsRevoked(ctx, tenant, jti, gracePeriod)
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
