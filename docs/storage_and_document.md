# Document Definition

## 1 Document (Internal Storage Type)

This is an internal type, visible only to the storage layer. Its usage should be strictly limited to the storage layer.

```go

// Document represents a stored document in the database
type Document struct {
	// Id is the unique identifier for the document, 128-bit BLAKE3 of fullpath, binary, (e.g., blake3("chats/chatroom-1/members/alice")[:16])
	Id string `json:"id" bson:"_id"`

  // Fullpath is the Full Pathname of document, "<collection>/<document>" , e.g., chats/chatroom-1/members/alice
  Fullpath string `json:"fullpath" bson:"fullpath"`

	// Collection is the collection with fullpath, e.g., chats/chatroom-1/members
	Collection string `json:"collection" bson:"collection"`

  // Parent is the parent of collection, e.g., chats/chatroom-1
  Parent string `json:"parent" bson:"parent"`

	// Data is the actual content of the document
	Data map[string]interface{} `json:"data" bson:"data"`

	// UpdatedAt is the timestamp of the last update (Unix millionseconds), Updated on every write
	UpdatedAt int64 `json:"updated_at" bson:"updated_at"`

	// CreatedAt is the timestamp of the creation (Unix millionseconds), Set on create only
	CreatedAt int64 `json:"created_at" bson:"created_at"`

	// Version is the optimistic concurrency control version, Auto-increment per update, client cannot set
	Version int64 `json:"version" bson:"version"`
}
```

## 2 User-Facing Document (Business Layer Type)

This is the business layer Document type, visible to the API.

```go
type Document map[string]interface{}
```

```json
{
  "id": "alice",               // Required; immutable once written; allowed charset: [A-Za-z0-9_.-]
  "collection": "chats/chatroom-1/members", // Shadow field, server-written; client input ignored
  "version": 0,                // Shadow field, server-written; client input ignored
  "updated_at": 1700000000000,  // Shadow field, server-written; client input ignored
  "created_at": 1700000000000,  // Shadow field, server-written; client input ignored
  /* other user fields */
}
```

**Rules & protections**
- `id` is required and immutable after creation; server enforces charset `[A-Za-z0-9_.-]` and rejects mutations on update/patch.
- Shadow fields (`collection`, `version`, `updated_at`, `created_at`) are server-owned; client-supplied values are ignored/overwritten.
- `version` in MongoDocument is incremented automatically per write and not client-settable.
- `created_at` is set once on create; `updated_at` updates on every write.
