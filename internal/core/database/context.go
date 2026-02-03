package database

import "context"

// contextKey is the type for context keys used by this package
type contextKey string

const (
	// contextKeyDatabase is the key for storing the resolved database in context
	contextKeyDatabase contextKey = "database"
)

// WithDatabase returns a new context with the database attached
func WithDatabase(ctx context.Context, db *Database) context.Context {
	return context.WithValue(ctx, contextKeyDatabase, db)
}

// FromContext retrieves the database from the context
// Returns the database and true if found, nil and false otherwise
func FromContext(ctx context.Context) (*Database, bool) {
	db, ok := ctx.Value(contextKeyDatabase).(*Database)
	return db, ok
}

// MustFromContext retrieves the database from the context
// Panics if the database is not found
func MustFromContext(ctx context.Context) *Database {
	db, ok := FromContext(ctx)
	if !ok {
		panic("database not found in context")
	}
	return db
}
