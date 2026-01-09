package mongo

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/syntrixbase/syntrix/internal/storage/types"
	"github.com/zeebo/blake3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type userStore struct {
	coll *mongo.Collection
}

func NewUserStore(db *mongo.Database, collectionName string) types.UserStore {
	if collectionName == "" {
		collectionName = "auth_users"
	}
	return &userStore{
		coll: db.Collection(collectionName),
	}
}

func (s *userStore) CreateUser(ctx context.Context, database string, user *types.User) error {
	// Ensure username is lowercase
	user.Username = strings.ToLower(user.Username)
	user.DatabaseID = database

	// Check if user exists
	filter := bson.M{"database_id": database, "username": user.Username}
	count, err := s.coll.CountDocuments(ctx, filter)
	if err != nil {
		return err
	}
	if count > 0 {
		return types.ErrUserExists
	}

	// Generate ID if empty
	if user.ID == "" {
		// Use database:hash(username)
		hash := blake3.Sum256([]byte(user.Username))
		user.ID = database + ":" + hex.EncodeToString(hash[:16])
	} else if !strings.HasPrefix(user.ID, database+":") {
		user.ID = database + ":" + user.ID
	}

	_, err = s.coll.InsertOne(ctx, user)
	return err
}

func (s *userStore) GetUserByUsername(ctx context.Context, database string, username string) (*types.User, error) {
	username = strings.ToLower(username)
	filter := bson.M{"database_id": database, "username": username}

	var user types.User
	err := s.coll.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, types.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (s *userStore) GetUserByID(ctx context.Context, database string, id string) (*types.User, error) {
	filter := bson.M{"_id": id, "database_id": database}

	var user types.User
	err := s.coll.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, types.ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (s *userStore) UpdateUserLoginStats(ctx context.Context, database string, id string, lastLogin time.Time, attempts int, lockoutUntil time.Time) error {
	filter := bson.M{"_id": id, "database_id": database}
	update := bson.M{
		"$set": bson.M{
			"last_login_at":  lastLogin,
			"login_attempts": attempts,
			"lockout_until":  lockoutUntil,
		},
	}
	_, err := s.coll.UpdateOne(ctx, filter, update)
	return err
}

func (s *userStore) ListUsers(ctx context.Context, database string, limit int, offset int) ([]*types.User, error) {
	opts := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	cursor, err := s.coll.Find(ctx, bson.M{"database_id": database}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var users []*types.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *userStore) UpdateUser(ctx context.Context, database string, user *types.User) error {
	filter := bson.M{"_id": user.ID, "database_id": database}
	update := bson.M{
		"$set": bson.M{
			"roles":      user.Roles,
			"disabled":   user.Disabled,
			"updated_at": time.Now(),
		},
	}
	_, err := s.coll.UpdateOne(ctx, filter, update)
	return err
}

func (s *userStore) EnsureIndexes(ctx context.Context) error {
	// User username unique index per database
	_, err := s.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "database_id", Value: 1}, {Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (s *userStore) Close(ctx context.Context) error {
	return nil
}
