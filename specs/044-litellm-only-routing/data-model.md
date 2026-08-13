# Phase 1 Data Model: 044-litellm-only-routing

**Date**: 2026-08-12

Most of this feature has no persistent data. Three things do: the embedding columns, the routing
catalogue (a file, but a contract with entities), and the configuration surface.

---

## 1. Persistent schema

### Migration `00044_embedding_dims_1024.sql`

| Object | Before | After |
|---|---|---|
| `"Job"."embedding"` | `vector(768)` | `vector(1024)`, contents discarded |
| `"Job"."embeddingHash"` | text, populated | nulled — this is what re-embeds every job lazily |
| `"Job"."embedModel"` | — | **new** `text`, nullable |
| `"Profile"."embedding"` | `vector(768)` | `vector(1024)`, contents discarded |
| `"Profile"."embedModel"` | text, populated | nulled |

```sql
-- +goose Up
ALTER TABLE "Job"     ALTER COLUMN "embedding" TYPE vector(1024) USING NULL;
ALTER TABLE "Profile" ALTER COLUMN "embedding" TYPE vector(1024) USING NULL;
ALTER TABLE "Job"     ADD COLUMN "embedModel" text;
UPDATE "Job"     SET "embeddingHash" = NULL;
UPDATE "Profile" SET "embedModel" = NULL;

-- +goose Down
ALTER TABLE "Job"     ALTER COLUMN "embedding" TYPE vector(768) USING NULL;
ALTER TABLE "Profile" ALTER COLUMN "embedding" TYPE vector(768) USING NULL;
ALTER TABLE "Job"     DROP COLUMN "embedModel";
UPDATE "Job"     SET "embeddingHash" = NULL;
UPDATE "Profile" SET "embedModel" = NULL;
```

`USING NULL` is the whole trick: Postgres will not retype a populated `vector(n)` column to a
different `n`, and there is nothing worth preserving (research.md R5). The down migration nulls
again rather than pretending to restore 768-dimension vectors it does not have.

**Version number**: `00044`, following `00043_drop_tailored_drafts.sql`. Sequential and unique per
the constitution's migration rule.

### Query changes

| Query | Change |
|---|---|
| `UpdateJobEmbedding` (`job.sql:19`) | gains `"embedModel" = $3` |
| `UpdateJobEmbeddingWithHash` (`job.sql:24`) | gains `"embedModel" = $4` |
| `UpdateProfileEmbedding` (`profile.sql:31`) | unchanged — already writes `embedModel` |
| **new** `ClearStaleJobEmbeddings` | `UPDATE "Job" SET "embedding" = NULL, "embeddingHash" = NULL WHERE "embedModel" IS DISTINCT FROM $1` — the operator-invocable form of FR-020 for a future model change |

Regenerate with sqlc; do not hand-edit `internal/db/sqlcgen/`.

---

## 2. Entities that live in `gateway/config.yaml`

Not database rows, but they have fields, invariants and a validating parser
(`internal/platform/llm/gateway_config_test.go`), so they are modelled here.

### Scenario

| Field | Meaning | Validation |
|---|---|---|
| `name` | The only identifier the application sends as `model` | MUST be requested by exactly one router in `compose.go`, or be a declared fallback tier |
| chain | Ordered tier list from `litellm_settings.fallbacks` | ≥2 tiers, ≥2 distinct providers, no `local`, no undeclared tier name |
| class | economy-structured / quality-writing / tool-capable / embedding | recorded as a comment beside the group; the ordering rule (FR-011) is checked by review, not by parser |

**Scenario set after this feature** — `match`, `ghost`, `rephrase`, `recruiter`, `salary`,
`outreach`, `generation`, `generation-analyze`, `generation-select`, `generation-select-premium`,
`generation-summary`, `generation-summary-premium`, `generation-summary-fast`, `embed`.
**Removed**: `default`, `local`.

### Tier

| Field | Meaning | Validation |
|---|---|---|
| `model` | provider/model string | non-empty |
| `api_key` | credential reference | MUST be `os.environ/…`; a literal fails the build |
| `model_info.supports_function_calling` | tool-capability claim | REQUIRED on every tier of `salary`; documentation a test reads, not an enforced control |
| deliberation bound | `reasoning_effort` / `reasoning.enabled` | REQUIRED on every tier whose family deliberates; absence is a config error |
| `output_dimension` | embedding width | REQUIRED and identical on every tier of `embed`; MUST equal the application's `EMBED_DIMS` |
| `input_type` | Cohere embedding mode | `search_document` on every `embed` tier (research.md R2) |

### Chain invariant, restated

The old invariant — *every chain terminates at `local`* — is replaced by:

> Every requested scenario resolves to a chain of **at least two tiers drawn from at least two
> distinct providers**, none of which is a self-hosted runtime.

`gateway_config_test.go` changes from asserting the terminal tier to asserting arity and provider
diversity, plus the per-class declarations above.

---

## 3. Configuration surface

### Required (startup fails without)

| Key | Note |
|---|---|
| `GATEWAY_URL` | previously optional; its emptiness selected the local path |
| `LITELLM_MASTER_KEY` | previously only required when `GATEWAY_URL` was set |

### Kept, with changed meaning

| Key | Change |
|---|---|
| `EMBED_DIMS` | default `768` → `1024`; was read by nothing, now asserted against every returned vector |
| `EMBED_MODEL` → `EMBED_MODEL_ID` | no longer names a model to call — it records which model the gateway is pinned to, is stored as embedding provenance, and is asserted against `gateway/config.yaml` by the guardrail test |
| `AI_CONCURRENCY_CLOUD` | now the only AI concurrency; name kept to avoid breaking deployed `.env` files |

### Removed

`OLLAMA_URL`, `OLLAMA_KEY`, `OLLAMA_KEEP_ALIVE`, `EMBED_URL`, `LLM_MODEL`, `LLM_MODEL_MATCH`,
`LLM_MODEL_GENERATION`, `LLM_MODEL_GENERATION_ANALYZE`, `LLM_MODEL_GENERATION_SELECT`,
`LLM_MODEL_GENERATION_PREMIUM`, `LLM_MODEL_GENERATION_SUMMARY`, `LLM_MODEL_REPHRASE`,
`LLM_MODEL_GHOST`, `AI_CONCURRENCY_LOCAL`.

Removed from `config.Config` and from `defaults.go` in the same change, so an operator who sets one
gets an unknown-key report rather than silence. `Config.ModelOr` and `Config.GenerationModelOr`
(`config.go:131,142`) are deleted with their only inputs.

### Gateway-side credentials (unchanged location: the `litellm` service only)

`CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY`, and **new**
`OPENAI_API_KEY` (the `embed` chain's second tier, research.md R4). `OLLAMA_URL`/`OLLAMA_KEY` are
removed from the `litellm` service environment too.

---

## 4. In-memory types

| Type | Change |
|---|---|
| `application.Router` | loses `local domain.Provider` and `localModel string`; `NewRouter(taskKey string, gateway domain.Provider)` |
| `application.ProviderClass` | kept; `ProviderClass()` returns `hosted` unconditionally |
| `queue.TaskPolicy` | `LocalConcurrency`+`HostedConcurrency` → `Concurrency`; `PoolSize()` returns it |
| `llmHandles` (`compose.go:258`) | loses `Ollama`; `DefaultRouter` → `SalaryRouter`, `OutreachRouter`, `RecruiterRouter` |
| `generation/domain.SummaryOption` | `SelfHosted()` and the `local` entry removed; `TaskKey` becomes non-empty for every option |
| `dto.QueueBacklogDto` | unchanged shape; `providerClass` is now always `"hosted"` for LLM queues |

## 5. Stored values that outlive their meaning

| Value | Where | Behaviour after this change |
|---|---|---|
| `"local"` summary-option id | `Document`/`GenerationRun` rows, the summary-model setting | `LookupSummaryOption` misses and returns the default (`summary_option.go`), so runs continue on `generation-summary`. Pinned by a test rather than left to be rediscovered. |
| 768-dimension vectors | `Job`/`Profile` | discarded by the migration; re-embedded lazily |
| `Job."embeddingHash"` | `Job` | nulled once; the mechanism itself is retained and keeps working |
