# Trigger Refactor Requirements

## Why

- Align trigger subsystem with storage-style factory/provider pattern to hide backend/NATS/HTTP details from callers.
- Reduce coupling so trigger engine can swap transport or worker implementations without touching service consumers.
- Improve testability with clear interfaces and injectable fakes.

## Goals

- Provide a single trigger entrypoint (engine) exposing only minimal lifecycle and trigger-loading APIs.
- Encapsulate NATS, HTTP worker, and storage watch/checkpoint details behind internal adapters.
- Keep configuration centralized and declarative (mirrors storage config style).
- Maintain current behaviors (event filtering, CEL evaluation, retry/backoff semantics) while clarifying boundaries.
- Enforce tenant isolation via subject prefixing on publish/consume and tenant-scoped checkpoints so resume tokens do not leak across tenants.

## Non-Goals

- Changing business semantics of trigger evaluation, retry math, or CEL language.
- Reworking storage.Event or storage.Document shapes.
- Introducing new external dependencies.

## Constraints

- Follow existing doc structure and naming; code and docs in English.
- Tests must continue using testify; no integration tests that bypass public interfaces.
- Preserve compatibility with current NATS subject format and worker HTTP contract unless explicitly revised later.
- NATS subject safety: docKey portion must be subject-safe (base64url without padding); publisher/consumer share the same encoding/decoding path.
- Secrets/tokens are tenant-scoped; cross-tenant reuse is forbidden.
- Shared `TRIGGERS` stream is assumed; monitor per-tenant traffic/retention and be ready to pivot to per-tenant streams if quotas/noisy-neighbor risks emerge.

## Observability baseline

- Emit per-tenant metrics for publish/consume/worker success, retry counts, and latency; tracing is optional but recommended for incident drills.
- Enforce subject-length guardrails: if encoded subject would exceed NATS limits, hash the docKey portion for the subject while keeping the original (decoded) docKey in the payload for worker use.
- Metrics shape: at minimum expose counters for publish/consume/worker success and retries, and histograms for end-to-end latency, all tagged with tenant and collection.
- Subject-length guard details: use NATS subject max of 1024 bytes; if `triggers.<tenant>.<collection>.<encodedDocKey>` exceeds this, replace `encodedDocKey` with `sha256(<encodedDocKey>)` hex truncated to 32 chars; always carry the original decoded docKey in the payload.
- Hash fallback flagging: when subject hashing occurs, set a boolean marker in payload/metrics (e.g., `subjectHashed=true`) to aid debugging and collision investigations.

## Known Issues (Dec 2025 review)

- Replication pull gap: current query engine uses `updatedAt > checkpoint`, so multiple docs with identical `updatedAt` can cause later docs to be skipped. The pull filter should be inclusive on the sort keys (e.g., `updatedAt >= checkpoint` with a secondary key such as `id`) to guarantee complete replication.
- Push input validation: when neither `Fullpath` nor `id` is present in an incoming change, the query engine will operate on an empty path. Validation must reject such requests before calling storage to avoid undefined writes or conflicts.
- Watch cancellation/timeout: `WatchCollection` relies on the default `http.Client` (no timeout) and does not break out of the decode loop on context cancellation, so long-lived half-open connections can leak goroutines. Add client timeouts and context-aware read loops.
- Delete conflict masking: in replication push delete flow, `ErrNotFound` is treated as success even when a `BaseVersion` was supplied, so version conflicts on already-deleted docs are hidden. Should surface conflict (return latest) when delete preconditions fail.
- Watch error observability: JSON decode errors in `WatchCollection` only log and then close the channel without signaling an error, so consumers cannot distinguish graceful completion from stream corruption; this can drop events silently. Should propagate errors to callers.
- Loader error signaling gap: `TriggerEngine.LoadTriggers` currently returns no error, yet tenant/collection/docKey validation is required to fail fast. Without an error channel, invalid triggers can be skipped silently. Why: callers cannot know load outcome, leading to partial activation and hard-to-debug misses.
- Hash-guard partitioning ambiguity: subject hashing replaces the docKey segment for long subjects, but consumer partitioning/idem-potency keys must use the decoded docKey from payload, not the hashed subject segment. Why: hashing alters routing keys; if partitioning uses the hashed segment, different docs can collide, affecting ordering and dedupe.
- Hash observability gap: requirements mention `subjectHashed=true`, but metrics/alerts for hash-path traffic are not mandated. Why: without counters/percentiles for hashed subjects (publish/consume/retry) and collision logging, hash-triggered routing issues are invisible.
- Checkpoint reset risk: "start from now" on missing checkpoint skips historical events by default. Why: accidental checkpoint loss would silently drop backlog; safer default is explicit opt-in or audit + metric when starting from now.
- Hash collision handling TBD (low priority): when subject hashing is used and hashed segment collides with an in-flight key, the handling path (queue, serialize, or fail) and alert thresholds are not defined. Why: even very low-probability collisions need a diagnosable handling path.
- Hash branch retry/alert tuning TBD (low priority): hash path is higher-risk but does not specify stricter retry caps/timeouts or alert windows. Why: without differentiated SLOs, hash-path anomalies may be detected late.
- "Start from now" audit/RBAC TBD (low priority): the audit sink (log/metric/table) and who can authorize this action are not specified. Why: without authorization and audit, skipping historical events is not traceable.
- LoadTriggers error migration guidance TBD (low priority): interface now returns error but caller migration steps (startup fail-fast, health/exit semantics) are not documented. Why: without migration guidance, callers may ignore errors or fail to exit when validation fails.
- Secret/token failure observability TBD (low priority): minimal metrics/log fields (tenant, secret ref, error class) for fetch/mint failures are not mandated. Why: lacking labeled metrics slows incident diagnosis.

## Open Questions

- Multi-tenant isolation: decided to keep a single `TRIGGERS` stream with subject prefixing (`triggers.<tenant>.>`) for routing; revisit only if quotas/retention require per-tenant streams.
- Checkpoints: must be tenant-scoped keys (e.g., `sys/checkpoints/trigger_evaluator/<tenant>`) to avoid cross-tenant resume token sharing.
- Observability: none beyond the above baseline; tracing hooks are nice-to-have.
