# SDK Upgrade Plan (Chat App)

## 1) Context

- Why: We have a new `@syntrix/client` SDK that unifies external app access and trigger worker access; current chat app uses a bespoke REST helper and manual fetch-based sync.
- How: Capture the migration approach so the worker and frontend can switch with minimal regression while keeping current nested agent flows intact.

## 2) Goals & Non-Goals

- Goals: adopt `@syntrix/client` for trigger workers (TriggerClient via TriggerHandler) and for the frontend (SyntrixClient); reduce custom HTTP glue; keep existing nested-agent data model and UX.
- Non-Goals: redesign UX, change collection schema, or alter orchestration logic.

## 3) Scope (Phased)

- Phase 1: Trigger Worker migration (webhooks -> TriggerHandler/TriggerClient). Remove legacy `syntrix-client.ts` after handlers switch.
- Phase 2: Frontend migration (RxDB sync adapter or direct client usage). Decide retain-or-drop RxDB offline cache.
- Phase 3: Clean-up & docs/tests hardening.

## 4) Delta vs Current

- Current: custom `SyntrixClient` hitting `/api/v1/trigger/query|get|write`, manual path assembly, no batch semantics, token taken from `preIssuedToken` manually.
- New: `@syntrix/client` exposes `TriggerHandler` (extracts `preIssuedToken`), `TriggerClient` with fluent `doc/collection/query/batch`, and standard `SyntrixClient` for external apps. Cleaner error surfaces and server-generated IDs (standard client only).

## 5) Data Model Impact

- No schema changes planned. Collections remain: `messages`, `tool-calls`, `tasks`, `sub-agents`, `sub-agents/*/messages`, `sub-agents/*/tool-calls`, `runs` (idempotency lock), etc.
- Path building will use `doc().collection()` instead of string concat; ensure we preserve explicit IDs for trigger writes.

## 6) Migration Plan — Trigger Worker (Phase 1)

- Handler bootstrap: in each Express handler, instantiate `const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL); const syntrix = handler.syntrix;` Replace bespoke client import.
- Reads: replace `query()` calls with `syntrix.collection(path).where(...).orderBy(...).get()`; `getDocument` -> `syntrix.doc(path).get()`.
- Writes: replace `createDocument`/`updateDocument` with `doc(path).set()` or `.update()`; group sequential writes into `batch()` where atomicity helps (e.g., sub-agent creation + task update + bootstrap messages in `agent-init`).
- Locks: preserve run-lock creation in `runs` by using `set` with deterministic IDs; handle conflict via error catching; verify SDK conflict code path.
- Tool calls: continue writing explicit IDs from LLM tool_calls; ensure `batch` ordering preserves assistant message then tool-call docs.
- Auth: rely on `TriggerHandler` to inject `preIssuedToken`; reject when absent (keep current 401 behavior).
- Cleanup: delete `src/syntrix-client.ts` and adjust imports once all handlers migrated.

## 7) Migration Plan — Frontend (Phase 2)

- Client factory: create a `getSyntrixClient(token)` helper wrapping `new SyntrixClient(API_URL, token)`.
- RxDB decision:
  - If keep RxDB: write a thin adapter that reads/writes via `SyntrixClient` instead of raw fetch; preserve local schema, but reduce duplication of fetchWithAuth logic. Real-time stays via RxDB replication; if the SDK lacks push, use existing replication HTTP sync.
  - If drop RxDB: replace replication with direct `collection().where().orderBy().get()` reads and `collection().add()/doc().update()` writes; add lightweight state management; explicitly use polling for realtime (default 2s, configurable).
- Incremental collection migration: start with `messages`/`tool-calls` (unified naming), then `tasks`/`sub-agents`, finally inspector views; keep schemas stable while swapping the data access layer.

## 8) Testing Strategy

- Worker unit tests: mock `TriggerHandler`/`TriggerClient`; cover happy paths and guardrails (missing token -> 401, pending tools skip, run-lock conflict, batch failures). Use testify per repository norm.
- Frontend tests: bun+vitest/RTL for the new client wrapper and auth-refresh flow; ensure queries/writes surface errors cleanly.
- Integration: minimal smoke by hitting a local Syntrix instance once migration compiles; avoid forbidden direct component calls per policy.

## 9) Risks / Mitigations

- Conflict semantics may differ: validate how `set` handles existing docs; keep deterministic IDs to avoid silent overwrites.
- Batch error handling: ensure partial failures bubble; consider transactional grouping only where necessary.
- RxDB parity: if keeping RxDB, adapter complexity could grow; bias toward simplifying to direct client if offline is non-critical.

## 10) Open Questions

- Do we need offline-first (keep RxDB), or can we simplify to direct client calls?
- Confirm target `@syntrix/client` version and any breaking base URL/auth header changes.
- Should we expose a thin domain repository layer to reduce path string repetition?

## 11) Decisions & Constraints (to reduce ambiguity)

- SDK version: pin `@syntrix/client` to the provided target version (if unspecified, pick the latest stable at upgrade time and record it here). `SYNTRIX_API_URL` must include `/api/v1` (consistent with current manual calls); example `http://localhost:8080/api/v1`.
- RxDB stance: Phase 2 keeps RxDB by default; if we drop offline mode, same PR must state we switch to polling (default 2s, configurable) and remove RxDB dependency. If kept, reuse RxDB replication for realtime.
- Client lifecycle: after token refresh, always recreate the client instance and rebind any RxDB adapter (do not rely on header injection) to avoid diverging state.
- IDs and writes: trigger-side always uses explicit IDs (`doc(id).set/update`); disable `collection().add()`. Tool-call collection name is unified as `tool-calls`.
- Batch semantics: treat batch as ordered but non-transactional; if partial failure occurs, surface the error and avoid assuming full success (no implicit compensation).
- Lock/conflict handling: run-lock conflicts surfaced as HTTP 409 (axios error.response.status == 409) are treated as benign duplicates; return success semantics, raise others.
- Collection naming: use `tool-calls` everywhere; before migration, scan code/data and remove `toolcall` variants.
- Testing minimums (Phase 1): missing `preIssuedToken` -> 401; pending tool -> skip; run-lock 409 -> skip; batch partial failure -> error; happy path for tool_calls + final_answer. Phase 2: token refresh recreates client; basic CRUD; errors bubble to UI; polling/replication path validated.
- Environment: `.env.example` lists `SYNTRIX_API_URL` (with `/api/v1`), `AZURE_OPENAI_ENDPOINT`, `AZURE_OPENAI_API_KEY`, `AZURE_OPENAI_MODEL`, worker `PORT`, and notes server must emit `preIssuedToken` in webhook payloads.
- Paths: collection/doc paths stay `users/{uid}/orch-chats/{cid}/...` unchanged.

## 12) Environment Example

```bash
# Syntrix server (include /api/v1)
SYNTRIX_API_URL=http://localhost:8080/api/v1

# Trigger worker
PORT=3000

# Azure OpenAI
AZURE_OPENAI_ENDPOINT=https://<your-endpoint>.openai.azure.com
AZURE_OPENAI_API_KEY=your-key
AZURE_OPENAI_MODEL=gpt-4o

# Webhook security
# Ensure server/webhook config enables preIssuedToken in payloads
```

## 13) Next Steps

- Decide RxDB keep/drop.
- Lock the SDK version and add it to both packages.
- Migrate one handler (e.g., `agent-init`) as a spike, add tests, then roll out to the rest.
- Plan the frontend data-layer swap after the worker side is stable.
