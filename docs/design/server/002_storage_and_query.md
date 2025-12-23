# Storage & Query Engine Discussion

**Date:** December 13, 2025
**Topic:** Storage Interface, Data Model, and Query DSL

## 1. Storage Interface (Go)

We need a `StorageBackend` interface that abstracts the underlying database (MongoDB). This allows us to potentially switch backends in the future and makes testing easier.

```go
package storage

import (
    "context"
)

// Document represents a stored document
// This is an internal type, visible only to the storage layer. Its usage should be strictly limited to the storage layer.
type Document struct {
    // Id is the unique identifier for the document, 128-bit BLAKE3 of fullpath, binary
    Id string `json:"id" bson:"_id"`

    // Fullpath is the Full Pathname of document
    Fullpath string `json:"fullpath" bson:"fullpath"`

    // Collection is the parent collection name
    Collection string `json:"collection" bson:"collection"`

    // Parent is the parent of collection
    Parent string `json:"parent" bson:"parent"`

    // Data is the actual content of the document
    Data map[string]interface{} `json:"data" bson:"data"`

    // UpdatedAt is the timestamp of the last update (Unix millionseconds)
    UpdatedAt int64 `json:"updatedAt" bson:"updatedAt"`

    // CreatedAt is the timestamp of the creation (Unix millionseconds)
    CreatedAt int64 `json:"createdAt" bson:"createdAt"`

    // Version is the optimistic concurrency control version
    Version int64 `json:"version" bson:"version"`

    // Deleted indicates if the document is soft-deleted
    Deleted bool `json:"deleted,omitempty" bson:"deleted,omitempty"`
}

// StorageBackend defines the interface for storage operations
type StorageBackend interface {
    // Get retrieves a document by its path
    Get(ctx context.Context, path string) (*Document, error)

    // Create inserts a new document. Fails if it already exists.
    Create(ctx context.Context, doc *Document) error

    // Update updates an existing document.
    // If pred is provided, it performs a CAS (Compare-And-Swap) operation.
    Update(ctx context.Context, path string, data map[string]interface{}, pred Filters) error

    // Patch updates specific fields of an existing document.
    // If pred is provided, it performs a CAS (Compare-And-Swap) operation.
    Patch(ctx context.Context, path string, data map[string]interface{}, pred Filters) error

    // Delete removes a document by its path
    Delete(ctx context.Context, path string, pred Filters) error

    // Query executes a complex query
    Query(ctx context.Context, q Query) ([]*Document, error)

    // Watch returns a channel of events for a given collection (or all if empty).
    // resumeToken can be nil to start from now.
    Watch(ctx context.Context, collection string, resumeToken interface{}, opts WatchOptions) (<-chan Event, error)

    // Transaction executes a function within a transaction.
    // The function receives a StorageBackend that is bound to the transaction.
    Transaction(ctx context.Context, fn func(ctx context.Context, tx StorageBackend) error) error

    // Close closes the connection to the backend
    Close(ctx context.Context) error
}
```

## 2. Data Model (MongoDB)

We will use a **Single Collection** strategy for simplicity and efficiency.

**Collection Name:** `documents`

**Schema:**

```javascript
{
  "_id": "binary_blake3_hash",              // Primary Key: 128-bit BLAKE3 of fullpath
  "fullpath": "users/alice/posts/post1",      // Full Pathname
  "collection": "users/alice/posts",          // Collection fullpath (for listing documents)
  "parent": "users/alice",                    // Parent path (parent of the collection)
  "updatedAt": NumberLong(1678888888000),    // Timestamp
  "createdAt": NumberLong(1678888888000),    // Timestamp
  "version": NumberLong(1),                   // Optimistic locking version
  "data": {                                   // Actual user data
    "id": "post1",
    "title": "Hello World",
    "content": "..."
  }
}
```

**Sharding Strategy:**

- **Shard Key**: `collection`.
- **Rationale**: Optimizes for read performance (Data Locality). Queries for a specific collection (e.g., "messages in room A") hit a single shard.
- **Trade-off**: Potential write hotspots for massive collections. Accepted for initial design.

**Indexes:**

1. `_id`: Default unique index.
2. `collection`: For listing documents in a collection (`db.documents.find({collection: "users/alice/posts"})`).
3. `parent`: For hierarchical queries (optional).
4. **Composite indexes**:
   - **Manual Creation**: Indexes must be created manually.
   - **Strict Mode**: Queries missing required indexes will **fail** and return an error suggesting the required index.

## 3. Query DSL

We need a structured way to represent queries that can be serialized (JSON) and passed from Client -> API -> Query Engine.

```go
// Query represents a database query
type Query struct {
    Collection  string  `json:"collection"`
    Filters     Filters `json:"filters"`
    OrderBy     []Order `json:"orderBy"`
    Limit       int     `json:"limit"`
    StartAfter  string  `json:"startAfter"` // Cursor (usually the last document ID or sort key)
    ShowDeleted bool    `json:"showDeleted"`
}

// Filter represents a query filter
type Filter struct {
    Field string      `json:"field"`
    Op    string      `json:"op"`
    Value interface{} `json:"value"`
}

// Order represents a sort order
type Order struct {
    Field     string `json:"field"`
    Direction string `json:"direction"` // "asc" or "desc"
}
```

### Query Translation (Example)

**Syntrix Query:**

```json
{
  "collection": "users/alice/posts",
  "filters": [
    {"field": "status", "op": "==", "value": "published"},
    {"field": "views", "op": ">", "value": 100}
  ],
  "orderBy": [{"field": "views", "direction": "desc"}],
  "limit": 10
}
```

**MongoDB Aggregation / Find:**

```javascript
db.documents.find({
  "parent": "users/alice/posts",
  "data.status": "published",
  "data.views": { "$gt": 100 }
}).sort({
  "data.views": -1
}).limit(10)
```

## 4. Open Questions

1. **Subcollections**: How do we handle deep nesting?
   - *Proposal*: The `parent` field handles this naturally. `users/alice` has parent `users`. `users/alice/posts/p1` has parent `users/alice/posts`.
2. **Array Contains**: How to support `array-contains` queries efficiently?
   - *Proposal*: MongoDB supports this natively. We just need to map the operator.
3. **Composite Indexes**: Do we require users to define indexes manually for complex queries (like Firestore)?
   - *Proposal*: Yes, initially. Auto-indexing is complex. We should fail queries that require a missing index.
