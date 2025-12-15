# Server-side Triggers & Webhooks Discussion

**Date:** December 15, 2025  
**Topic:** Database Triggers, Webhooks, and External Integrations

## 1) Architecture & Module Boundary
- Source: database change streams (CRUD events) feed trigger evaluation.
- Module: dedicated trigger-delivery service (separate from API/CSP/Gateway) handles outbound calls, retries, and DLQ to isolate latency/failures.
- Execution model: default asynchronous via durable queue + workers; synchronous (inline) only for narrow low-latency cases with strict timeout/circuit-breaker and per-tenant caps.
- Partitioning (Why: fairness and isolation; How):
  - Partition queue by (tenant, collection, docKey hash); cap outstanding work per partition to stop a single hot document from starving others.
- Storage (Why: durable handoff; How):
  - Use a log/queue with visibility timeouts (e.g., Kafka/NATS JetStream/Redis Streams); persist trigger evaluation output (condition match result + payload) before enqueue to decouple from CSP/Gateway.
- Resource limits (Why: prevent noisy neighbors and runaway payloads; How):
  - Max outbound payload 256 KB (body); headers capped at 16 KB; synchronous execution wall time capped at 500 ms, async worker request timeout 5 s; max retries per item 10 with jitter; per-tenant concurrency cap 64 workers (configurable).

Data flow (logical):
```
Change Stream -> Trigger Evaluator (condition match) -> Queue (partitioned by tenant/collection/hash) -> Trigger Delivery Workers -> Webhook endpoints / DLQ
```

## 2) Trigger Definition & Matching
- Conditions: reuse the CEL subset expression language (from 002) for trigger conditions.
- Events: create/update/delete; payload includes before/after snapshots, tenant, collection, docKey, lsn/seq.
- Idempotency: key = (tenant, triggerId, lsn/seq) to dedupe downstream.
- Config shape (Why: consistent rollout; How):
  - `triggerId`, `version`, `tenant`, `collection`, `events[]`, `condition`, `url`, `headers`, `secretsRef`, `mode` (async|inline), `concurrency`, `rateLimit`, `retryPolicy`, `ordering` (best-effort|strict), `filters` (optional path list).
- Evaluator (Why: safety and predictability; How):
  - Compile conditions once per config version; restrict CEL functions to deterministic ops; reject unbounded glob-style filters.
- Ordering modes (Why: make trade-offs explicit; How):
  - Best-effort: partitioned by (tenant, collection, docKey hash) allows parallel delivery; may reorder across partitions.
  - Strict: single-collection ordered mode forces a single in-order lane per collection; throughput expected to drop to ~10–20% of best-effort; only enable for small cardinality use cases.

## 3) Delivery & Reliability
- Semantics: at-least-once; workers dequeue, sign, deliver, and ack.
- Retries: exponential backoff with max attempts; DLQ on exhaustion or fatal responses (e.g., 4xx configurable).
- Ordering: best-effort per partition/document; optional “ordered” mode per collection with reduced throughput.
- Backpressure: per-tenant/per-trigger rate limits and concurrency caps to avoid noisy neighbors.
- Retry policy (Why: avoid thundering herds; How):
  - Jittered exponential backoff; classify responses: retry on 429/5xx/timeouts, drop or DLQ on explicit 4xx (configurable list).
- Poison handling (Why: avoid infinite retries; How):
  - Detect repeated fast-fail cycles; short-circuit to DLQ with a succinct error snapshot (status, headers hash, truncated body, attempt count).
- DLQ replay (Why: recovery path; How):
  - Replay requires operator action; replay preserves original idempotency key and appends a replay reason; support partial replay by tenant/trigger/time range.
  - Replay governance (Why: auditability; How): require operator identity + reason logged, dry-run preview of count/size before execution, and metrics on replay success/failure.

## 4) Webhook Security
- Transport: HTTPS required; optional outbound mTLS; optional fixed egress IPs if needed.
- Integrity: HMAC signature over body + timestamp (per-tenant secret); support secret rotation with overlap window.
- Size/timeout: cap request/response size; enforce timeouts; configurable headers; blocklist/allowlist for destinations if required.
- Secret handling (Why: least privilege; How):
  - Store secrets in a secret manager; workers fetch via short-lived tokens; keep dual-secret window during rotation; enforce clock-skew tolerance on timestamped signatures.
- Signature tolerance (Why: avoid false rejects; How): default timestamp skew tolerance ±300s; reject outside window with explicit error code for observability.

Example webhook payload + signature:
```json
{
  "triggerId": "trg_123",
  "tenant": "acme",
  "event": "update",
  "collection": "orders",
  "docKey": "abc123",
  "lsn": "1697041234:42",
  "seq": 99102,
  "before": { "status": "pending" },
  "after":  { "status": "shipped" },
  "ts": 1697041234
}
```
Header: `X-Syntrix-Signature: t=1697041234,v1=hex(hmac_sha256(secret, t + "." + body))`

## 5) Admin & Operations
- Management API/CLI: create/update triggers (url, events, condition, headers, auth), rotate secrets, view status and metrics, inspect/replay DLQ, test delivery (dry-run).
- Observability: per-trigger metrics (success/fail/latency, retry counts, DLQ size), structured logs with requestId/triggerId/tenant.
- Rollout: trigger configs are versioned; allow staged rollout/disable; health checks on endpoints before enabling.
- Audit (Why: traceability; How):
  - Record who changed what (user, time, diff) for trigger configs; emit change events to an audit log; require signatures for sensitive updates (e.g., URL/secret changes).
- Dry-run (Why: safe rollout; How):
  - Dry-run mode evaluates conditions and records would-be deliveries without sending; compare hit rates before enabling real delivery.
- Staged rollout (Why: reduce blast radius; How):
  - Support percentage- or tenant-scoped enablement; auto-roll back on error rate or latency SLO breach.

## 6) Future Extensibility
- Reserved path for sandboxed functions (JS/Go) running out-of-process with CPU/mem/time limits, reusing the same event model and delivery semantics.
- Transform hooks (Why: adapt payloads without webhook churn; How):
  - Allow lightweight, bounded transformations (e.g., field masks, templating) executed in the sandboxed function path with strict limits; keep default path passthrough for latency-sensitive triggers.
