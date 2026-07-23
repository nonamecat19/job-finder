# Phase 0 Research: Cerebras Free-Tier Model Toggle

## R1. Cerebras provider implementation

**Decision**: Port the existing legacy `cerebras.go` (found in worktree
`.claude/worktrees/agent-a05cfd63cf87db244/apps/api-go/internal/llm/cerebras.go`) into
`apps/api/internal/llm/cerebras.go`, implementing the current `llm.Provider` interface
(`ModelName`, `Complete`, `CompleteJSON`).

**Rationale**: It already targets Cerebras's OpenAI-compatible `POST {baseURL}/chat/completions`
with Bearer auth, default base `https://api.cerebras.ai/v1`, and delegates `Embed` to an
Ollama provider (Cerebras has no embeddings). It mirrors the retry/strip-fences behavior the
Go `llm` package already standardizes (`CompleteStructured`). Minimal new code; proven shape.

**Alternatives considered**: Generic OpenAI-compatible client — rejected as premature
abstraction; only Cerebras is in scope. A vendor SDK — rejected (extra dep; the REST shape is
trivial).

**Verify on port**: interface drift since the legacy copy (current `Provider` interface,
`CompleteOptions` fields `Model`/`MaxTokens`/`Temp`/`SystemPrompt`), and that structured JSON
uses `response_format` where supported.

## R2. Cerebras free-tier model list

**Decision**: Ship a curated, code-defined list of supported free-tier chat models with one
marked default (`gpt-oss-120b` per the legacy default). Represent as a small Go slice exposed
via the models endpoint; the dashboard renders it as a dropdown. Do not fetch the list live.

**Rationale**: Spec assumption "curated model list" — operators pick known-good options, not
free-text. A static list is testable and avoids a live dependency at settings-load time.
Candidate free-tier models (confirm current availability at implementation): `gpt-oss-120b`,
`llama-3.3-70b`, `llama3.1-8b`, `qwen-3-32b`. Final list confirmed against Cerebras docs in the
implementing task.

**Alternatives considered**: Live `GET /v1/models` — rejected: adds a network call + auth
dependency to render Settings, and free-tier eligibility isn't reliably derivable from it.
Free-text model field — rejected by spec (curated).

## R3. Credential handling (env-only)

**Decision**: Add `CEREBRAS_API_KEY` (secret, no default, optional key) and `CEREBRAS_BASE_URL`
(default `https://api.cerebras.ai/v1`) to `config.go` `optionalKeys`/`defaults`. The key is
read once at startup. The settings API exposes only a boolean `credentialConfigured`, never
the value.

**Rationale**: Matches Q1=B and FR-011/FR-013. Mirrors existing secret patterns (`OLLAMA_KEY`,
`JOOBLE_API_KEY`) that live in env and never reach the browser. No DB encryption needed since
nothing secret is persisted — simpler than the Djinni-cookie/`crypto` path.

**Alternatives considered**: Dashboard-entered key encrypted with `ConfigEncryptionKey`
(like Djinni) — rejected by Q1=B. Simpler and removes a secret-at-rest surface.

## R4. Runtime per-task resolution architecture

**Decision**: Introduce `llm.Router`. A `Router` is created per task key (`match`,
`generation`, `rephrase`, `ghost`, and a `default` bucket for other chat callers e.g. salary/
coach). It implements `llm.Provider`; on each call it reads the current setting for its task
from an in-memory snapshot and dispatches to the bound Ollama or Cerebras provider with the
resolved model. Services are injected a task-bound `Router` in place of `(provider, modelStr)`.

**Rationale**: Services already depend only on the `llm.Provider` interface and pass a model
string. A Router that *is* a Provider lets us satisfy FR-005 (runtime, no restart) and FR-014
(per-task) with near-zero change to service internals — constructors drop the model-string arg
and the `CompleteOptions.Model` override (the Router owns the model). Single seam, testable in
isolation.

**Alternatives considered**: Thread a resolver + task key through every `Complete` call site —
rejected: touches every service method signature. Rebuild providers on each settings change and
swap a global — rejected: races with in-flight tasks and needs restart-like coordination;
Router snapshot per-call is cleaner and matches "new work uses new setting, running work
finishes on its provider" (spec assumption).

## R5. Settings persistence + cache invalidation

**Decision**: New table `llm_task_setting` (one row per task key) storing `provider` and
`model`. `llmsettings.Service` loads all rows into an atomically-swapped in-memory snapshot at
startup and after every update (`PUT`). The Router reads the snapshot.

**Rationale**: Single-operator instance, tiny row count. In-memory snapshot keeps per-call
resolution free of DB latency; explicit reload on write keeps it consistent (FR-004/FR-005).
Postgres via sqlc keeps Constitution III/goose conventions.

**Alternatives considered**: Read DB per task call — rejected (needless latency on every LLM
call). Redis-backed — rejected (over-engineered for one small config record).

## R6. Missing-credential / error surfacing

**Decision**: On `PUT`, validate: setting a task to Cerebras while `credentialConfigured` is
false is accepted-but-inert only if we choose to; instead, per FR-008 the API keeps the task on
Ollama and returns `credentialConfigured:false` so the UI shows a "credential missing" banner.
At call time, if a Cerebras call fails (401/403/429/model error), the Router returns the error
up the existing error path; handlers surface it and it is visible in task status. FR-011: never
log the key; log provider/model/status only.

**Rationale**: Satisfies Story 3 + FR-008/FR-009 without a new error framework — reuse existing
error propagation and task-status surfacing.

**Alternatives considered**: Silent fallback to Ollama on Cerebras failure — rejected: hides
quota/credential problems the operator must see (FR-009, SC-005).

## R7. Migration numbering

**Decision**: `00018_llm_task_setting.sql` (next sequential; existing max is `00017`, with
`00013` intentionally absent). Unique, sequential per Constitution "Technology & Architecture
Constraints".

**Rationale**: Matches goose convention already in the repo; no reuse of a version number.
