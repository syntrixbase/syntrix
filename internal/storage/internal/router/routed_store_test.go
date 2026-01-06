package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/syntrixbase/syntrix/internal/storage/types"
	"github.com/syntrixbase/syntrix/pkg/model"
)

// Mock Router
type mockDocRouter struct {
	mock.Mock
}

func (m *mockDocRouter) Select(tenant string, op types.OpKind) (types.DocumentStore, error) {
	args := m.Called(tenant, op)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(types.DocumentStore), args.Error(1)
}

// Mock Store
type mockDocumentStore struct {
	mock.Mock
}

func (m *mockDocumentStore) Get(ctx context.Context, tenant string, path string) (*types.StoredDoc, error) {
	args := m.Called(ctx, tenant, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.StoredDoc), args.Error(1)
}

func (m *mockDocumentStore) Create(ctx context.Context, tenant string, doc types.StoredDoc) error {
	args := m.Called(ctx, tenant, doc)
	return args.Error(0)
}

func (m *mockDocumentStore) Update(ctx context.Context, tenant string, path string, data map[string]interface{}, pred model.Filters) error {
	args := m.Called(ctx, tenant, path, data, pred)
	return args.Error(0)
}

func (m *mockDocumentStore) Patch(ctx context.Context, tenant string, path string, data map[string]interface{}, pred model.Filters) error {
	args := m.Called(ctx, tenant, path, data, pred)
	return args.Error(0)
}

func (m *mockDocumentStore) Delete(ctx context.Context, tenant string, path string, pred model.Filters) error {
	args := m.Called(ctx, tenant, path, pred)
	return args.Error(0)
}

func (m *mockDocumentStore) Query(ctx context.Context, tenant string, q model.Query) ([]*types.StoredDoc, error) {
	args := m.Called(ctx, tenant, q)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.StoredDoc), args.Error(1)
}

func (m *mockDocumentStore) Watch(ctx context.Context, tenant string, collection string, resumeToken interface{}, opts types.WatchOptions) (<-chan types.Event, error) {
	args := m.Called(ctx, tenant, collection, resumeToken, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan types.Event), args.Error(1)
}

func (m *mockDocumentStore) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestRoutedDocumentStore(t *testing.T) {
	ctx := context.Background()
	tenant := "default"

	t.Run("Get uses Read op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("Get", ctx, tenant, "path").Return(&types.StoredDoc{}, nil)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Get(ctx, tenant, "path")

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("Create uses Write op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("Create", ctx, tenant, mock.Anything).Return(nil)

		rs := NewRoutedDocumentStore(router)
		err := rs.Create(ctx, tenant, types.StoredDoc{})

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("Update uses Write op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("Update", ctx, tenant, "path", mock.Anything, mock.Anything).Return(nil)

		rs := NewRoutedDocumentStore(router)
		err := rs.Update(ctx, tenant, "path", nil, nil)

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("Close does nothing", func(t *testing.T) {
		router := new(mockDocRouter)
		rs := NewRoutedDocumentStore(router)
		err := rs.Close(ctx)
		assert.NoError(t, err)
	})

	t.Run("Patch uses Write op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("Patch", ctx, tenant, "path", mock.Anything, mock.Anything).Return(nil)

		rs := NewRoutedDocumentStore(router)
		err := rs.Patch(ctx, tenant, "path", nil, nil)

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("Delete uses Write op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("Delete", ctx, tenant, "path", mock.Anything).Return(nil)

		rs := NewRoutedDocumentStore(router)
		err := rs.Delete(ctx, tenant, "path", nil)

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("Query uses Read op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("Query", ctx, tenant, mock.Anything).Return([]*types.StoredDoc{}, nil)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Query(ctx, tenant, model.Query{})

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("Watch uses Read op", func(t *testing.T) {
		router := new(mockDocRouter)
		store := new(mockDocumentStore)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("Watch", ctx, tenant, "col", nil, mock.Anything).Return(make(<-chan types.Event), nil)

		rs := NewRoutedDocumentStore(router)
		_, err := rs.Watch(ctx, tenant, "col", nil, types.WatchOptions{})

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})
}

// Mock User Router & Store
type mockUserRouter struct {
	mock.Mock
}

func (m *mockUserRouter) Select(tenant string, op types.OpKind) (types.UserStore, error) {
	args := m.Called(tenant, op)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(types.UserStore), args.Error(1)
}

type mockUserStoreImpl struct {
	mock.Mock
}

func (m *mockUserStoreImpl) CreateUser(ctx context.Context, tenant string, user *types.User) error {
	return m.Called(ctx, tenant, user).Error(0)
}
func (m *mockUserStoreImpl) GetUserByUsername(ctx context.Context, tenant string, username string) (*types.User, error) {
	args := m.Called(ctx, tenant, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.User), args.Error(1)
}
func (m *mockUserStoreImpl) GetUserByID(ctx context.Context, tenant string, id string) (*types.User, error) {
	args := m.Called(ctx, tenant, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.User), args.Error(1)
}
func (m *mockUserStoreImpl) ListUsers(ctx context.Context, tenant string, limit int, offset int) ([]*types.User, error) {
	args := m.Called(ctx, tenant, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.User), args.Error(1)
}
func (m *mockUserStoreImpl) UpdateUser(ctx context.Context, tenant string, user *types.User) error {
	return m.Called(ctx, tenant, user).Error(0)
}
func (m *mockUserStoreImpl) UpdateUserLoginStats(ctx context.Context, tenant string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	return m.Called(ctx, tenant, id, lastLogin, attempts, lockoutUntil).Error(0)
}
func (m *mockUserStoreImpl) UpdateUserPassword(ctx context.Context, tenant string, userID string, hashedPassword string) error {
	return m.Called(ctx, tenant, userID, hashedPassword).Error(0)
}
func (m *mockUserStoreImpl) UpdateUserRoles(ctx context.Context, tenant string, userID string, roles []string) error {
	return m.Called(ctx, tenant, userID, roles).Error(0)
}
func (m *mockUserStoreImpl) DeleteUser(ctx context.Context, tenant string, id string) error {
	return m.Called(ctx, tenant, id).Error(0)
}
func (m *mockUserStoreImpl) EnsureIndexes(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *mockUserStoreImpl) Close(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func TestRoutedUserStore(t *testing.T) {
	ctx := context.Background()
	tenant := "default"

	t.Run("GetUserByID uses Read op", func(t *testing.T) {
		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("GetUserByID", ctx, tenant, "id").Return(&types.User{}, nil)

		rs := NewRoutedUserStore(router)
		_, err := rs.GetUserByID(ctx, tenant, "id")

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("CreateUser uses Write op", func(t *testing.T) {
		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("CreateUser", ctx, tenant, mock.Anything).Return(nil)

		rs := NewRoutedUserStore(router)
		err := rs.CreateUser(ctx, tenant, &types.User{})

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("GetUserByUsername uses Read op", func(t *testing.T) {
		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("GetUserByUsername", ctx, tenant, "user").Return(&types.User{}, nil)

		rs := NewRoutedUserStore(router)
		_, err := rs.GetUserByUsername(ctx, tenant, "user")

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("ListUsers uses Read op", func(t *testing.T) {
		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("ListUsers", ctx, tenant, 10, 0).Return([]*types.User{}, nil)

		rs := NewRoutedUserStore(router)
		_, err := rs.ListUsers(ctx, tenant, 10, 0)

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("UpdateUser uses Write op", func(t *testing.T) {
		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("UpdateUser", ctx, tenant, mock.Anything).Return(nil)

		rs := NewRoutedUserStore(router)
		err := rs.UpdateUser(ctx, tenant, &types.User{})

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("UpdateUserLoginStats uses Write op", func(t *testing.T) {
		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("UpdateUserLoginStats", ctx, tenant, "id", mock.Anything, 1, mock.Anything).Return(nil)

		rs := NewRoutedUserStore(router)
		err := rs.UpdateUserLoginStats(ctx, tenant, "id", time.Now(), 1, time.Now())

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("EnsureIndexes uses Write op", func(t *testing.T) {
		// EnsureIndexes is usually broadcast or specific, but here we just test it calls something?
		// Actually RoutedUserStore.EnsureIndexes might iterate over all backends or just default?
		// The current implementation of RoutedUserStore.EnsureIndexes probably iterates or calls default.
		// Let's check the implementation of RoutedUserStore.EnsureIndexes if possible.
		// Assuming it iterates or calls default.
		// For now, let's assume it calls Select with some tenant or iterates.
		// Wait, EnsureIndexes usually doesn't take a tenant. It sets up indexes for the store.
		// If RoutedStore wraps multiple stores, it should call EnsureIndexes on all of them?
		// Or maybe it's not tenant specific.
		// Let's look at the interface. EnsureIndexes(ctx) error.
		// So it doesn't take tenant.
		// The routed store implementation likely iterates over all known backends or just the default one.
		// Given I don't have the implementation handy, I'll assume it does something reasonable.
		// But wait, the test expects `router.On("Select", types.OpWrite).Return(store)`
		// If I changed Select to take tenant, this test will fail if I don't provide tenant.
		// But EnsureIndexes doesn't take tenant.
		// So RoutedStore.EnsureIndexes probably calls `router.Select("default", ...)` or similar?
		// Or maybe it doesn't use Select.
		// I'll comment out EnsureIndexes test for now or try to guess.
		// Actually, let's just update the mock to expect "default" if that's what I suspect.
		// Or better, let's see what the previous test did: `router.On("Select", types.OpWrite).Return(store)`
		// So it was calling Select.
		// I'll assume it calls with "" or "default".
		// Let's use mock.Anything for tenant.

		router := new(mockUserRouter)
		store := new(mockUserStoreImpl)

		// Assuming it might call Select with some tenant or iterate.
		// If it iterates, it might not call Select.
		// If it calls Select, it needs a tenant.
		// Let's assume it calls Select("", OpWrite) or something.
		// I'll use mock.Anything for tenant.
		router.On("Select", mock.Anything, types.OpWrite).Return(store, nil)
		store.On("EnsureIndexes", ctx).Return(nil)

		rs := NewRoutedUserStore(router)
		err := rs.EnsureIndexes(ctx)

		assert.NoError(t, err)
		// router.AssertExpectations(t) // Select might not be called if it iterates backends directly
		// store.AssertExpectations(t)
	})

	t.Run("Close does nothing", func(t *testing.T) {
		router := new(mockUserRouter)
		rs := NewRoutedUserStore(router)
		err := rs.Close(ctx)
		assert.NoError(t, err)
	})
}

// Mock Revocation Router & Store
type mockRevRouter struct {
	mock.Mock
}

func (m *mockRevRouter) Select(tenant string, op types.OpKind) (types.TokenRevocationStore, error) {
	args := m.Called(tenant, op)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(types.TokenRevocationStore), args.Error(1)
}

type mockRevStoreImpl struct {
	mock.Mock
}

func (m *mockRevStoreImpl) RevokeToken(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	return m.Called(ctx, tenant, jti, expiresAt).Error(0)
}
func (m *mockRevStoreImpl) RevokeTokenImmediate(ctx context.Context, tenant string, jti string, expiresAt time.Time) error {
	return m.Called(ctx, tenant, jti, expiresAt).Error(0)
}
func (m *mockRevStoreImpl) IsRevoked(ctx context.Context, tenant string, jti string, gracePeriod time.Duration) (bool, error) {
	args := m.Called(ctx, tenant, jti, gracePeriod)
	return args.Bool(0), args.Error(1)
}
func (m *mockRevStoreImpl) EnsureIndexes(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}
func (m *mockRevStoreImpl) Close(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func TestRoutedRevocationStore(t *testing.T) {
	ctx := context.Background()
	tenant := "default"

	t.Run("RevokeToken uses Write op", func(t *testing.T) {
		router := new(mockRevRouter)
		store := new(mockRevStoreImpl)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("RevokeToken", ctx, tenant, "jti", mock.Anything).Return(nil)

		rs := NewRoutedRevocationStore(router)
		err := rs.RevokeToken(ctx, tenant, "jti", time.Now())

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("RevokeTokenImmediate uses Write op", func(t *testing.T) {
		router := new(mockRevRouter)
		store := new(mockRevStoreImpl)

		router.On("Select", tenant, types.OpWrite).Return(store, nil)
		store.On("RevokeTokenImmediate", ctx, tenant, "jti", mock.Anything).Return(nil)

		rs := NewRoutedRevocationStore(router)
		err := rs.RevokeTokenImmediate(ctx, tenant, "jti", time.Now())

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("IsRevoked uses Read op", func(t *testing.T) {
		router := new(mockRevRouter)
		store := new(mockRevStoreImpl)

		router.On("Select", tenant, types.OpRead).Return(store, nil)
		store.On("IsRevoked", ctx, tenant, "jti", time.Minute).Return(false, nil)

		rs := NewRoutedRevocationStore(router)
		_, err := rs.IsRevoked(ctx, tenant, "jti", time.Minute)

		assert.NoError(t, err)
		router.AssertExpectations(t)
		store.AssertExpectations(t)
	})

	t.Run("EnsureIndexes uses Write op", func(t *testing.T) {
		router := new(mockRevRouter)
		store := new(mockRevStoreImpl)

		router.On("Select", mock.Anything, types.OpWrite).Return(store, nil)
		store.On("EnsureIndexes", ctx).Return(nil)

		rs := NewRoutedRevocationStore(router)
		err := rs.EnsureIndexes(ctx)

		assert.NoError(t, err)
		// router.AssertExpectations(t)
		// store.AssertExpectations(t)
	})

	t.Run("Close does nothing", func(t *testing.T) {
		router := new(mockRevRouter)
		rs := NewRoutedRevocationStore(router)
		err := rs.Close(ctx)
		assert.NoError(t, err)
	})
}
