# Authorization Rules Discussion

**Date:** December 14, 2025
**Topic:** Data-Driven Authorization (Security Rules)

## 1. Overview

To support complex access control scenarios like private chat rooms, we are moving from simple Collection-level RBAC to a **Data-Driven Attribute-Based Access Control (ABAC)** system. This is inspired by Firestore Security Rules and CEL (Common Expression Language).

## 2. Rule Model

Rules are defined separately from the code and loaded by the API Gateway. They evaluate whether a request should be allowed or denied based on:
1.  **Request**: Who is asking? (`request.auth`)
2.  **Resource**: What is being accessed? (`resource.data` for reads)
3.  **Future State**: What will the data look like? (`request.resource.data` for writes)

### 2.1 Structure

Rules are hierarchical, matching the document path structure.

```yaml
rules_version: '1'
service: syntrix

match:
  /databases/{database}/documents:
    match: /users/{userId}:
      allow: read, write: if request.auth.uid == userId

    match: /rooms/{roomId}:
      allow: read: if resource.data.public == true || request.auth.uid in resource.data.members
      allow: write: if request.auth.uid in resource.data.members

      match: /messages/{messageId}:
        allow: read: if get(/databases/$(database)/documents/rooms/$(roomId)).data.members.has(request.auth.uid)
        allow: create: if get(/databases/$(database)/documents/rooms/$(roomId)).data.members.has(request.auth.uid)
```

## 3. Evaluation Context

The rules engine will have access to the following objects:

### `request`
- `request.auth.uid`: The authenticated user's ID.
- `request.auth.token`: The full JWT token claims.
- `request.time`: Server timestamp.
- `request.resource`: The new resource data (for `create` and `update` operations).

### `resource`
- `resource.data`: The *existing* document data (for `read`, `update`, `delete`).
- `resource.id`: The document ID.

### Helper Functions
- `get(path)`: Fetches a document from the database (crucial for checking parent permissions, e.g., room membership).
- `exists(path)`: Checks if a document exists.

## 4. Implementation Strategy

### 4.1 Language
We will use **CEL (Common Expression Language)** by Google. It is fast, safe, and designed for this exact purpose. Go has excellent support for CEL (`github.com/google/cel-go`).

### 4.2 Execution Flow (API Gateway)

1.  **Incoming Request**: `GET /v1/rooms/123/messages/msg1`
2.  **Auth Check**: Validate JWT, extract `uid`.
3.  **Rule Match**: Find the rule matching path `/rooms/123/messages/msg1`.
4.  **Pre-fetch (Optimization)**:
    - If rule uses `resource.data`, fetch the target document.
    - If rule uses `get()`, fetch the referenced documents (with caching).
5.  **Evaluate**: Run the CEL program.
6.  **Decision**:
    - `true`: Proceed to Query Engine.
    - `false`: Return `403 Forbidden`.

## 5. Performance Considerations

- **`get()` Cost**: Rules that require fetching other documents (e.g., checking room membership for every message read) are expensive.
- **Optimization**:
    - **JWT Claims**: Embed common permissions (e.g., `role: admin`) in the JWT to avoid DB lookups.
    - **Denormalization**: Duplicate `memberIds` array into the message document (trade-off: storage vs. compute).
    - **Caching**: Cache `get()` results within the scope of a request or short-term.

## 6. Example Scenarios

### Scenario A: User Profile
*Only the user can edit their own profile. Public can read.*

```cel
match /users/{userId} {
  allow read: if true;
  allow write: if request.auth.uid == userId;
}
```

### Scenario B: Private Chat Room
*Only members can read/write messages.*

**Option 1: Parent Lookup (Normalized)**
```cel
match /rooms/{roomId}/messages/{msgId} {
  allow read, write: if request.auth.uid in get(/databases/$(database)/documents/rooms/$(roomId)).data.members
}
```

**Option 2: Denormalized (Faster)**
*Requires `members` array to be copied to every message.*
```cel
match /rooms/{roomId}/messages/{msgId} {
  allow read: if request.auth.uid in resource.data.members
}
```
