# Query Engine & DSL

**Date:** December 25, 2025
**Topic:** Query representation, translation, and examples

## 1) Query Model (wire shape)
```go
// Query represents a database query
type Query struct {
    Collection  string  `json:"collection"`
    Filters     Filters `json:"filters"`
    OrderBy     []Order `json:"orderBy"`
    Limit       int     `json:"limit"`
    StartAfter  string  `json:"startAfter"` // cursor
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

## 2) Translation Example
- Syntrix Query:
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
- Mongo Find:
```javascript
db.documents.find({
  "parent": "users/alice/posts",
  "data.status": "published",
  "data.views": { "$gt": 100 }
}).sort({ "data.views": -1 }).limit(10)
```

## 3) Execution Semantics
- Query Engine consumes `DocumentStore` (interface) and outputs flattened documents (user-visible fields plus metadata: id, collection, version, timestamps, deleted flag).
- Reserved fields (`id`, `collection`, `version`, `updatedAt`, `createdAt`, `deleted`) are added/stripped at the service layer; storage keeps internal representation.
- Missing index → error with suggested index (align with storage/index policy).
- Pagination via `startAfter` cursor; `ShowDeleted` enables tombstone reads when needed (e.g., replication).

## 4) Open Questions
- Index suggestion schema (to align with storage planner).
- Cursor shape (string vs struct) and stability across backend types.
- CEL/rules pushdown: which filters can be pushed vs must be post-filtered.
