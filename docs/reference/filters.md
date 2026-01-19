# Query Filters Guide

Syntrix supports a specific set of operators for filtering documents in queries and realtime subscriptions.

## Supported Operators

The following operators are supported:

| Operator | Description | Example |
|----------|-------------|---------|
| `==` | Equality | `{"field": "status", "op": "==", "value": "active"}` |
| `!=` | Not equal | `{"field": "status", "op": "!=", "value": "inactive"}` |
| `>` | Greater than | `{"field": "age", "op": ">", "value": 18}` |
| `>=` | Greater than or equal | `{"field": "age", "op": ">=", "value": 18}` |
| `<` | Less than | `{"field": "score", "op": "<", "value": 100}` |
| `<=` | Less than or equal | `{"field": "score", "op": "<=", "value": 100}` |
| `in` | Value is in a list | `{"field": "role", "op": "in", "value": ["admin", "editor"]}` |
| `contains` | Array contains value | `{"field": "tags", "op": "contains", "value": "news"}` |

## Performance Considerations

- The `!=` operator is supported but may be less efficient for indexing compared to equality or range queries. When possible, consider using alternative query structures or filtering on the client side for better performance.

## Usage Examples

### Simple Equality
```json
{
  "collection": "users",
  "filters": [
    {"field": "active", "op": "==", "value": true}
  ]
}
```

### Not Equal
```json
{
  "collection": "users",
  "filters": [
    {"field": "status", "op": "!=", "value": "deleted"}
  ]
}
```

### Range Query
```json
{
  "collection": "products",
  "filters": [
    {"field": "price", "op": ">=", "value": 10},
    {"field": "price", "op": "<=", "value": 100}
  ]
}
```

### Array Membership
```json
{
  "collection": "posts",
  "filters": [
    {"field": "tags", "op": "contains", "value": "news"}
  ]
}
```

### In Query
```json
{
  "collection": "users",
  "filters": [
    {"field": "status", "op": "in", "value": ["online", "away"]}
  ]
}
```
