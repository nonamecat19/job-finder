# Contract: LLM Settings API

Mounted at `/v1/settings/llm` within the shared `/api` subrouter (chi `Mount`, same pattern
as `ExtAuthHandler`), so the client-facing path is `/api/v1/settings/llm` (and
`/api/v1/settings/llm/models`) — the dashboard's `api.ts` fetch helper already prefixes
every call with `/api`. All DTOs are
defined in `apps/api/internal/dto/dto.go` and regenerated to `packages/shared/src/generated.ts`
via tygo (Constitution III). Frontend calls via `api.settings.*` in `apps/dashboard/src/lib/api.ts`.

Task keys: `match`, `generation`, `rephrase`, `ghost`, `default`.
Providers: `ollama`, `cerebras`.

## GET /v1/settings/llm

Returns current per-task settings + provider availability status.

**200 Response**
```json
{
  "credentialConfigured": true,
  "tasks": [
    { "taskKey": "match",      "provider": "ollama",   "model": "" },
    { "taskKey": "generation", "provider": "cerebras", "model": "gpt-oss-120b" },
    { "taskKey": "rephrase",   "provider": "ollama",   "model": "" },
    { "taskKey": "ghost",      "provider": "ollama",   "model": "" },
    { "taskKey": "default",    "provider": "ollama",   "model": "" }
  ]
}
```
- `credentialConfigured`: true iff `CEREBRAS_API_KEY` is set at startup. The key value is
  NEVER returned.
- `model` empty string means "provider default".

## GET /v1/settings/llm/models

Returns the curated Cerebras free-tier model list for the dropdown.

**200 Response**
```json
{
  "cerebras": [
    { "id": "gpt-oss-120b", "label": "GPT-OSS 120B", "isDefault": true },
    { "id": "llama-3.3-70b", "label": "Llama 3.3 70B", "isDefault": false },
    { "id": "llama3.1-8b",  "label": "Llama 3.1 8B",  "isDefault": false }
  ]
}
```
(Final list confirmed against Cerebras docs at implementation — see research R2.)

## PUT /v1/settings/llm

Update one or more task assignments. Body may include any subset of tasks; omitted tasks are
unchanged. Supports the "switch all" case by sending all tasks with `provider=cerebras`.

**Request**
```json
{
  "tasks": [
    { "taskKey": "generation", "provider": "cerebras", "model": "gpt-oss-120b" },
    { "taskKey": "match",      "provider": "ollama",   "model": "" }
  ]
}
```

**Validation**
- `taskKey` MUST be a known key; unknown → `400`.
- `provider` MUST be `ollama` or `cerebras`; else → `400`.
- If `provider=cerebras` and `model` is non-empty, it SHOULD match a curated model id;
  unknown id → `400` (or accepted with a warning — implementer picks; default: `400`).
- Setting `provider=cerebras` is allowed even when `credentialConfigured=false`; the write
  persists and the response's `credentialConfigured=false` signals the UI to warn (FR-008).
  The Router keeps such a task on Ollama until a key is configured.

**200 Response**: same shape as `GET /v1/settings/llm` (full current state after update).
The in-memory Router snapshot is reloaded before responding, so the change is live for
newly started tasks (FR-005).

**Errors**
- `400` invalid task key / provider / model.
- `500` persistence failure (settings unchanged).

## Behavior at task execution (not an HTTP contract, stated for completeness)

- Router resolves `{provider, model}` per task from the current snapshot.
- Cerebras selected + credential missing → task runs on Ollama; status reflects the fallback
  reason (FR-008).
- Cerebras call fails (401/403/429/model error) → error propagates to the task's existing
  error surface with an actionable message (FR-009); credential value never logged (FR-011).
- Embeddings always use the Ollama embedding endpoint regardless of task provider (FR-006).
