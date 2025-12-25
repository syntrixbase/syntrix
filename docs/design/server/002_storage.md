# Storage Design (Interfaces, Backend Abstraction, Data Model)

**Date:** December 25, 2025
**Topic:** Storage interfaces, backend abstraction, data model, routing

## 1) Goals & Principles
- Hide backend details (Mongo/PG/Redis…) behind interfaces; upper layers never see driver types.
- Support per-store backend choice (Document, User, Revocation can differ) via config-driven assembly.
- Enable routing policies (single, dual-write, read-replica) without changing call sites.
- Keep data model and indexes stable; enforce index requirements explicitly.

## 2) Interfaces (public surface, no driver types)
- **DocumentStore**: `Get`, `Create`, `Update`, `Patch`, `Delete`, `Query`, `Watch`, `Close`.
- **UserStore**: `CreateUser`, `GetUserByUsername`, `GetUserByID`, `UpdateUserLoginStats`, `ListUsers`, `UpdateUser`, `EnsureIndexes`, `Close`.
- **TokenRevocationStore**: `RevokeToken`, `RevokeTokenImmediate`, `IsRevoked`, `EnsureIndexes`, `Close`.
- Optional **AuthStore** aggregation: `Users() UserStore`, `Revocations() TokenRevocationStore` (constructor convenience).

## 3) Provider / Registry (config-driven)
- Single external entry: `storage.NewFromConfig(cfg.Storage)` → returns a registry `{Document(), Users(), Revocations(), Close()}`.
- Inside storage: compose one or more backend providers (e.g., Mongo for docs, PG for users, Redis for revocation) based on config; upper layers are unaware of backend types.
- Providers own connections/config and call `EnsureIndexes` during init; registry `Close` cascades to child stores.

## 4) Routing Strategy (read/write split & migration)
- Strategy interface (sketch): `SelectDocument(op)`, `SelectUser(op)`, `SelectRevocation(op)` where `op` encodes read/write/migrate.
- Defaults: `single` strategy (reads=writes=primary) for all stores.
- Migration-ready: allow `dual-write` (primary+shadow) and `read-replica` (reads shadow, writes primary) per store via config.

## 5) Configuration (example)
```yaml
storage:
  document:
    backend: mongo
    mongo:
      uri: ...
      db: ...
      dataCollection: ...
      sysCollection: ...
      softDeleteRetention: ...
  user:
    backend: pg
    pg:
      dsn: ...
  revocation:
    backend: redis
    redis:
      addr: ...
      db: 0
      prefix: syntrix:rev
  routing:
    document:
      strategy: single        # single | dual-write | read-replica
      primary: mongo
      shadow: pg-doc          # optional (for migration)
      reads: primary          # primary | shadow | both
      writes: primary         # primary | both
    user:
      strategy: single
      primary: pg
    revocation:
      strategy: single
      primary: redis
```

## 6) Data Model (Mongo-oriented, storage-internal)
- Single-collection strategy for documents (data collection name configurable).
- Schema fields: `_id` (BLAKE3(fullpath) 128-bit), `fullpath`, `collection`, `parent`, `updatedAt`, `createdAt`, `version`, `data`, `deleted`.
- Sharding (Mongo): shard key `collection` for locality; accept hotspot trade-off.
- Indexes:
  1) `_id` unique (default).
  2) `collection` for listing.
  3) `parent` for hierarchy (optional).
  4) Composite indexes: created manually; missing required index → fail query with suggestion.

## 7) Watch & Replication (bridge to CSP/clients)
- `Watch(collection, resumeToken, opts)` returns events with `ResumeToken` opaque to callers.
- Replication uses flattened docs at the API layer; storage keeps internal shape and does not leak `_id/fullpath/parent`.

## 8) Open Items
- Index suggestion format for missing indexes (to align with query planner).
- Exact routing op taxonomy (read/write/bulk/migrate) to finalize before implementation.
- Additional backends: keep registry open for new providers (e.g., SQLite for local dev).
