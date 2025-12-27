package mongo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codetrek/syntrix/internal/storage/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func setupTestUserStore(t *testing.T) (types.UserStore, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(testMongoURI))
	require.NoError(t, err)

	// Ping to ensure connection
	err = client.Ping(ctx, nil)
	if err != nil {
		t.Skip("MongoDB not available, skipping integration tests")
	}

	db := client.Database(testDBName)

	// Clean up
	err = db.Drop(ctx)
	require.NoError(t, err)

	store := NewUserStore(db, "")
	err = store.EnsureIndexes(ctx)
	require.NoError(t, err)

	return store, func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	}
}

func TestUserStore_UserLifecycle(t *testing.T) {
	s, teardown := setupTestUserStore(t)
	defer teardown()

	ctx := context.Background()
	tenant := "default"

	user := &types.User{
		ID:           "user1",
		Username:     "TestUser",
		PasswordHash: "hash",
		CreatedAt:    time.Now().Truncate(time.Millisecond), // Truncate for mongo precision
		UpdatedAt:    time.Now().Truncate(time.Millisecond),
	}

	// 1. Create User
	err := s.CreateUser(ctx, tenant, user)
	require.NoError(t, err)

	// 2. Create Duplicate User (should fail)
	err = s.CreateUser(ctx, tenant, user)
	assert.ErrorIs(t, err, types.ErrUserExists)

	// 3. Get User By Username (case insensitive)
	fetched, err := s.GetUserByUsername(ctx, tenant, "testuser")
	require.NoError(t, err)
	assert.Equal(t, user.ID, fetched.ID)
	assert.Equal(t, "testuser", fetched.Username) // Should be stored lowercase

	// 4. Get User By ID
	fetchedID, err := s.GetUserByID(ctx, tenant, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "testuser", fetchedID.Username)

	// 5. Get Non-existent User
	_, err = s.GetUserByUsername(ctx, tenant, "nonexistent")
	assert.ErrorIs(t, err, types.ErrUserNotFound)

	_, err = s.GetUserByID(ctx, tenant, "nonexistent")
	assert.ErrorIs(t, err, types.ErrUserNotFound)

	// 6. Update Login Stats
	now := time.Now().Truncate(time.Millisecond)
	lockout := now.Add(1 * time.Hour)
	err = s.UpdateUserLoginStats(ctx, tenant, user.ID, now, 5, lockout)
	require.NoError(t, err)

	fetchedUpdated, err := s.GetUserByID(ctx, tenant, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, fetchedUpdated.LoginAttempts)
	assert.Equal(t, now.UnixMilli(), fetchedUpdated.LastLoginAt.UnixMilli())
	assert.Equal(t, lockout.UnixMilli(), fetchedUpdated.LockoutUntil.UnixMilli())
}

func TestUserStore_ListUsersAndUpdate(t *testing.T) {
	s, teardown := setupTestUserStore(t)
	defer teardown()

	ctx := context.Background()
	tenant := "default"

	baseTime := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	users := []*types.User{
		{ID: "u1", Username: "Alice", Roles: []string{"reader"}, CreatedAt: baseTime, UpdatedAt: baseTime},
		{ID: "u2", Username: "Bob", Roles: []string{"writer"}, CreatedAt: baseTime, UpdatedAt: baseTime},
		{ID: "u3", Username: "Carol", Roles: []string{"admin"}, CreatedAt: baseTime, UpdatedAt: baseTime},
	}

	for _, u := range users {
		require.NoError(t, s.CreateUser(ctx, tenant, u))
	}

	firstPage, err := s.ListUsers(ctx, tenant, 2, 0)
	require.NoError(t, err)
	assert.Len(t, firstPage, 2)

	secondPage, err := s.ListUsers(ctx, tenant, 2, 2)
	require.NoError(t, err)
	assert.Len(t, secondPage, 1)

	allUsers := append(firstPage, secondPage...)
	idSet := map[string]struct{}{}
	for _, u := range allUsers {
		idSet[u.ID] = struct{}{}
		assert.Equal(t, strings.ToLower(u.Username), u.Username)
	}
	assert.Len(t, idSet, 3)

	// Use the ID from the created user object, which now has the tenant prefix
	targetID := users[1].ID
	original, err := s.GetUserByID(ctx, tenant, targetID)
	require.NoError(t, err)

	update := &types.User{ID: targetID, Roles: []string{"admin", "editor"}, Disabled: true}
	require.NoError(t, s.UpdateUser(ctx, tenant, update))

	updated, err := s.GetUserByID(ctx, tenant, targetID)
	require.NoError(t, err)
	assert.Equal(t, []string{"admin", "editor"}, updated.Roles)
	assert.True(t, updated.Disabled)
	assert.True(t, updated.UpdatedAt.After(original.UpdatedAt))
}
