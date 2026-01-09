package authn

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) CreateUser(ctx context.Context, database string, user *User) error {
	args := m.Called(ctx, database, user)
	return args.Error(0)
}

func (m *MockStorage) GetUserByUsername(ctx context.Context, database string, username string) (*User, error) {
	args := m.Called(ctx, database, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockStorage) GetUserByID(ctx context.Context, database string, id string) (*User, error) {
	args := m.Called(ctx, database, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockStorage) UpdateUserLoginStats(ctx context.Context, database string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	args := m.Called(ctx, database, id, lastLogin, attempts, lockoutUntil)
	return args.Error(0)
}

func (m *MockStorage) RevokeToken(ctx context.Context, database string, jti string, expiresAt time.Time) error {
	args := m.Called(ctx, database, jti, expiresAt)
	return args.Error(0)
}

func (m *MockStorage) RevokeTokenImmediate(ctx context.Context, database string, jti string, expiresAt time.Time) error {
	args := m.Called(ctx, database, jti, expiresAt)
	return args.Error(0)
}

func (m *MockStorage) IsRevoked(ctx context.Context, database string, jti string, gracePeriod time.Duration) (bool, error) {
	args := m.Called(ctx, database, jti, gracePeriod)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) ListUsers(ctx context.Context, database string, limit int, offset int) ([]*User, error) {
	args := m.Called(ctx, database, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*User), args.Error(1)
}

func (m *MockStorage) UpdateUser(ctx context.Context, database string, user *User) error {
	args := m.Called(ctx, database, user)
	return args.Error(0)
}

func (m *MockStorage) EnsureIndexes(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Close(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}
