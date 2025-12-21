# TypeScript Client SDK Reference

The `@syntrix/client` package provides a type-safe, fluent interface for interacting with Syntrix. It includes two distinct clients: `SyntrixClient` for external applications and `TriggerClient` for internal trigger workers.

## Installation

```bash
npm install @syntrix/client
# or
bun add @syntrix/client
```

## 1. SyntrixClient (Standard)

Use this client in your external applications (Web, Mobile, Backend).

### Initialization

```typescript
import { SyntrixClient } from '@syntrix/client';

const client = new SyntrixClient('<URL_ENDPOINT>', 'YOUR_ACCESS_TOKEN');
```

### Methods

#### `doc<T>(path: string): DocumentReference<T>`
Creates a reference to a specific document.

#### `collection<T>(path: string): CollectionReference<T>`
Creates a reference to a collection.

---

## 2. TriggerClient (Internal)

Use this client **only** within Syntrix Trigger Workers. It requires the `preIssuedToken` from the webhook payload.

### Initialization (via TriggerHandler)

The recommended way to initialize is using the `TriggerHandler` helper.

```typescript
import { TriggerHandler, WebhookPayload } from '@syntrix/client';

// Inside your Express/HTTP handler
const payload = req.body as WebhookPayload;
const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL);
const client = handler.syntrix; // Returns TriggerClient
```

### Exclusive Methods

#### `batch(writes: WriteOp[]): Promise<void>`
Performs an atomic batch of write operations.

```typescript
await client.batch([
  { type: 'create', path: 'users/123', data: { name: 'Alice' } },
  { type: 'update', path: 'stats/daily', data: { count: 1 } }
]);
```

---

## 3. Fluent API (Shared)

Both clients return `DocumentReference` and `CollectionReference` objects that share the same API.

### DocumentReference `<T>`

Represents a single document.

*   **`get(): Promise<T | null>`**
    Fetch the document. Returns `null` if not found.
    ```typescript
    const doc = await client.doc('users/alice').get();
    ```

*   **`set(data: T): Promise<T>`**
    Overwrite the document (equivalent to HTTP PUT).
    ```typescript
    await client.doc('users/alice').set({ name: 'Alice', age: 30 });
    ```

*   **`update(data: Partial<T>): Promise<T>`**
    Partially update the document (equivalent to HTTP PATCH).
    ```typescript
    await client.doc('users/alice').update({ age: 31 });
    ```

*   **`delete(): Promise<void>`**
    Delete the document.
    ```typescript
    await client.doc('users/alice').delete();
    ```

*   **`collection(path: string): CollectionReference`**
    Get a sub-collection reference.
    ```typescript
    const posts = client.doc('users/alice').collection('posts');
    ```

### CollectionReference `<T>`

Represents a collection of documents.

*   **`doc(id: string): DocumentReference<T>`**
    Get a reference to a specific document in this collection.

*   **`add(data: T, id?: string): Promise<DocumentReference<T>>`**
    Create a new document. If `id` is omitted, the server will generate one (Standard Client only).
    ```typescript
    const newDocRef = await client.collection('posts').add({ title: 'New Post' });
    ```

*   **`get(): Promise<T[]>`**
    Fetch all documents in the collection (default limit applies).

*   **`where(field: string, op: string, value: any): QueryBuilder<T>`**
    Start a query.
    ```typescript
    const activeUsers = await client.collection('users')
        .where('status', '==', 'active')
        .get();
    ```

*   **`orderBy(field: string, direction: 'asc' | 'desc'): QueryBuilder<T>`**
    Sort results.

*   **`limit(n: number): QueryBuilder<T>`**
    Limit the number of results.

---

## 4. Types

### `WebhookPayload`
The payload received by a Trigger Worker.

```typescript
interface WebhookPayload<T = any> {
    triggerId: string;
    event: 'create' | 'update' | 'delete';
    collection: string;
    preIssuedToken?: string; // Critical for TriggerClient
    before?: T;
    after?: T;
    // ... other fields
}
```
