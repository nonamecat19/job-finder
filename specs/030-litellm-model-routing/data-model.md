# Phase 1 Data Model: Gateway-Owned Model Routing

This feature is net-negative on persisted state: it deletes one table and adds none. The "model" below is mostly configuration and in-process values.

## 1. AI Task (unchanged, now the only routing input)

Fixed set of five keys — the vocabulary shared by the Go workers, the queue policies, and the gateway config.

| Key | Consumers | JSON output required |
|-----|-----------|----------------------|
| `match` | matching worker/service | yes (structured fit score) |
| `generation` | resume/cover-letter generation | yes (structured document sections) |
| `rephrase` | keyword-diff rephrase suggester | yes |
| `ghost` | ghost-job detector | yes |
| `default` | salary inference, recruiter, outreach, coach | yes for salary; free-form elsewhere |

**Invariants**
- The set is closed; adding a key requires a code change *and* a matching group in `gateway/config.yaml`.
- The task key is the entire routing request: it is sent as the OpenAI `model` field and carries no provider or model identity (FR-004).
- Every key MUST exist as a group in the gateway config; a request for an unknown group is a loud 400 from the proxy, not a silent default.

## 2. Routing Chain (configuration, not data)

Ordered list of tiers attempted for one task.

```
<task> → tier₁ (free) → tier₂ (free) → tier₃ (free) → tier₄ (aggregator) → tier₅ (local)
```

**Fields per tier**: group name, provider prefix, model ID, credential reference (`os.environ/...`), optional `api_base`.

**Validation rules**
- Every free-tier entry MUST precede every OpenRouter entry (FR-006).
- The last entry of every chain MUST be the local/Ollama deployment (FR-008).
- Every model in a JSON-consuming task's chain MUST support `response_format: json_object` (research R3).
- Chains may share tier groups across tasks when the same model suits both.

**State transitions** (per deployment, owned by the proxy)
```
healthy --(allowed_fails failures)--> cooled-down --(cooldown_time elapsed)--> healthy
```
A cooled-down deployment is skipped without an attempt; the chain advances to the next tier.

## 3. Provider Credential

| Variable | Consumed by | Absent behaviour |
|----------|-------------|------------------|
| `CEREBRAS_API_KEY` | litellm container | tier fails auth → chain advances |
| `GROQ_API_KEY` | litellm container | same |
| `COHERE_API_KEY` | litellm container | same |
| `OPENROUTER_API_KEY` | litellm container | same |
| `OLLAMA_URL`, `OLLAMA_KEY` | litellm container **and** the Go app | app-side: local fallback path |
| `LITELLM_MASTER_KEY` | litellm + Go app | required whenever `GATEWAY_URL` is set |
| `GATEWAY_URL` | Go app only | empty → app talks to Ollama directly (FR-009) |

**Invariants**
- Credentials are environment-only. They are never persisted, never returned by any endpoint, never rendered in the dashboard (FR-010).
- Every provider key is declared in the litellm service environment with an empty default so config load never fails on an undefined variable (research R2).

## 4. Task Router (in-process, replaces the snapshot Router)

One instance per task key, constructed once at startup.

| Field | Value |
|-------|-------|
| `taskKey` | one of the five keys; sent as the request model when routing through the gateway |
| `primary` | gateway provider, or nil when `GATEWAY_URL` is empty |
| `local` | Ollama provider (always non-nil) |
| `localModel` | per-task `LLM_MODEL_*` value, used only on the direct-Ollama path |

**Behaviour**
- `Complete`/`CompleteJSON` → `primary` when non-nil (model = `taskKey`), else `local` (model = `localModel`, falling back to `LLM_MODEL`).
- `Embed` → always `local` (FR-014).
- `ProviderClass()` → `hosted` when `primary != nil`, else `local.IsHosted()`.
- Immutable after construction: no snapshot, no atomic swap, no runtime reconfiguration path.

## 5. REMOVED — Per-Task Provider Assignment

**Table** `"LlmTaskSetting"` (`taskKey` PK, `provider`, `model`, `updatedAt`), introduced in `00018_llm_task_setting.sql` and amended by `00020_openrouter_provider.sql`.

**Migration** `00033_drop_llm_task_setting.sql`
- Up: `DROP TABLE IF EXISTS "LlmTaskSetting";`
- Down: recreate the table with the 00020-era provider CHECK and re-seed the five task rows at `provider='ollama', model=''`.

**Cascading removals**: `internal/db/queries/llmsetting.sql`, the generated `sqlcgen` model and query methods, `internal/llmsettings/**`, DTOs `LlmTaskSettingDto` / `LlmSettingsResponseDto` / `UpdateLlmSettingsRequestDto` / `CerebrasModelDto` / `LlmModelsResponseDto`, and their `packages/shared` counterparts. Exhaustive list in [contracts/removed-surface.md](./contracts/removed-surface.md).
