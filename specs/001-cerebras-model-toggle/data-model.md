# Phase 1 Data Model: Cerebras Free-Tier Model Toggle

## Entity: LLM Task Setting

Persisted, instance-wide. One row per chat task key. Holds the provider and model each task
resolves to. **No credential is stored here** (credential is env-only).

**Table**: `llm_task_setting`

| Field        | Type        | Notes |
|--------------|-------------|-------|
| task_key     | text (PK)   | One of: `match`, `generation`, `rephrase`, `ghost`, `default`. Fixed enum-by-convention. |
| provider     | text        | `ollama` or `cerebras`. NOT NULL. Default `ollama`. |
| model        | text        | Model name for the chosen provider. Empty string = "provider default model". |
| updated_at   | timestamptz | NOT NULL default now(); set on update. |

**Constraints / rules**:
- `provider` MUST be `ollama` or `cerebras` (CHECK constraint).
- `task_key` is a closed set; rows are seeded for all five keys in the migration so reads are
  never missing a task.
- Empty `model` resolves to the provider's default model at call time (Ollama: `LLM_MODEL`
  env / provider default; Cerebras: curated default `gpt-oss-120b`).
- Setting `provider=cerebras` is persisted regardless of credential presence, but the Router
  only dispatches to Cerebras when a credential is configured; otherwise the operator sees a
  "credential missing" state (FR-008). (Implementers MAY instead reject the write — see
  contracts; default chosen: persist + surface status, so the choice is remembered once the
  key is added.)

**State transitions**: None beyond value updates. Last write wins (single operator).

## Derived / non-persisted: Supported Cerebras Model

Code-defined curated list (not a table). Returned by the models endpoint.

| Field    | Type    | Notes |
|----------|---------|-------|
| id       | string  | Cerebras model identifier passed as `model` (e.g. `gpt-oss-120b`). |
| label    | string  | Human display name. |
| isDefault| boolean | Exactly one true. |

## Relationships

- `LLM Task Setting.model` (when `provider=cerebras`) SHOULD be one of the Supported Cerebras
  Model ids, or empty (→ curated default). A previously selected id no longer offered falls
  back to the default at resolve time and the operator is informed (spec edge case).
- No FK relationships; both are small closed sets.

## Migration

`apps/api/internal/db/migrations/00018_llm_task_setting.sql` (goose up/down):
- `up`: create table + CHECK on provider; seed five rows (`match`, `generation`, `rephrase`,
  `ghost`, `default`) with `provider='ollama'`, `model=''`.
- `down`: drop table.

## sqlc queries (`internal/db/queries/llmsetting.sql`)

- `ListLlmTaskSettings` → all rows (startup + post-update snapshot load).
- `UpsertLlmTaskSetting(task_key, provider, model)` → set provider/model + `updated_at=now()`.
