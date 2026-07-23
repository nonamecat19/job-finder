# Implementation Plan: Cerebras Free-Tier Model Toggle

**Branch**: `001-cerebras-model-toggle` | **Date**: 2026-07-23 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-cerebras-model-toggle/spec.md`

## Summary

Add Cerebras as a first-class chat/completion provider alongside the existing Ollama
provider, and let the operator assign each of the four chat tasks (matching, generation,
rephrase, ghost-job) to Ollama or a Cerebras free-tier model from dashboard Settings — plus
a one-action "switch all". Selections persist in Postgres and take effect for newly started
tasks without a restart. The Cerebras API key is supplied only via deploy-time env
(`CEREBRAS_API_KEY`); embeddings stay on Ollama.

Technical approach: introduce a `CerebrasProvider` (port the existing legacy
`cerebras.go`, OpenAI-compatible `/v1/chat/completions`) and an `llm.Router` that resolves
`{provider, model}` per task at call time from a settings store cached in memory and backed
by a new `llm_task_setting` table. Services keep depending on the `llm.Provider` interface;
each is injected a task-bound `Router` view instead of the raw provider + baked model
string. Expose `GET/PUT /v1/settings/llm` and a supported-models/credential-status endpoint;
add a per-task provider/model matrix to the Settings page.

## Technical Context

**Language/Version**: Go 1.2x (apps/api), TypeScript/React 18 + Vite (apps/dashboard)

**Primary Dependencies**: chi router, sqlc (pgx/v5), goose migrations, viper config, asynq;
TanStack Query, Tailwind, tygo (Go→TS types), `@job-finder/shared`

**Storage**: PostgreSQL (pgvector). New table `llm_task_setting`. No secret stored in DB
(credential is env-only).

**Testing**: `go test` (api), `vitest` (dashboard); `make test-lint` for cross-app; live
Cerebras call guarded behind an env-gated smoke test (no key → skip)

**Target Platform**: Linux server via Docker Compose; single self-hosted operator

**Project Type**: Web application (Go API + React dashboard + Python sidecar)

**Performance Goals**: Provider/model resolution adds negligible latency (in-memory cached
settings, DB read only on cache miss/invalidate). No change to task throughput.

**Constraints**: Ollama stays the default; app must remain fully operational with zero
external AI calls when Cerebras is unset. Credential never reaches the browser or logs.

**Scale/Scope**: 1 operator, 4 named chat tasks + a default bucket, ~1 settings screen
section, 1 new provider, 1 migration, 1 router.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply**: Unaffected — no submission path touched. ✅
- **II. Grounded Generation**: Unaffected — same prompts/pipeline; only the backing
  provider/model changes. Grounding guarantees are provider-independent. ✅
- **III. Typed Contracts Across Boundaries**: New DTOs defined in `apps/api/internal/dto`
  and regenerated to `packages/shared/src/generated.ts` via tygo; DB access via sqlc-
  generated code from a new query file. No hand-duplicated types. ✅ (enforced in tasks)
- **IV. Test Discipline Per Language**: `go test` for router/provider/settings service +
  handler; `vitest` for Settings UI + hooks; env-gated live Cerebras smoke test; migration
  covered by the existing db integration harness. ✅
- **V. Local-First, Self-Hosted by Default**: ⚠ **Deviation, justified.** Cerebras is an
  external hosted API. This is compatible because it is strictly **opt-in and off by
  default** — fresh installs run 100% on local Ollama, and with no `CEREBRAS_API_KEY` the
  app is fully operational with zero external AI calls. The operator explicitly enables it,
  exactly as `LINKEDIN_SCRAPE_ENABLED` gates an opt-in external dependency today. Embeddings
  remain local. See Complexity Tracking. ✅ (documented)

Gate result: **PASS** (one documented deviation, no unjustified violations).

## Project Structure

### Documentation (this feature)

```text
specs/001-cerebras-model-toggle/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── llm-settings.md   # GET/PUT settings + models/status contract
└── tasks.md             # /speckit-tasks output (not created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── llm/
│   │   ├── cerebras.go            # NEW: CerebrasProvider (port legacy), Provider iface
│   │   ├── router.go             # NEW: task-bound Router resolving {provider,model}
│   │   ├── factory.go            # CHANGED: build Ollama + optional Cerebras + Router
│   │   └── ollama.go             # unchanged
│   ├── llmsettings/              # NEW: settings service (load/update, in-mem cache)
│   │   ├── service.go
│   │   └── service_test.go
│   ├── config/config.go          # CHANGED: add CEREBRAS_API_KEY, CEREBRAS_BASE_URL
│   ├── db/
│   │   ├── migrations/00018_llm_task_setting.sql   # NEW
│   │   └── queries/llmsetting.sql                   # NEW (sqlc)
│   ├── dto/dto.go                # CHANGED: LlmSettings DTOs (tygo source of truth)
│   └── httpapi/llm_settings.go   # NEW: handler, Mount at /v1/settings/llm
└── cmd/server/main.go            # CHANGED: wire settings svc + Router into services

apps/dashboard/src/
├── features/settings/
│   ├── SettingsPage.tsx          # CHANGED: add "AI models" section
│   ├── LlmSettingsCard.tsx       # NEW: per-task matrix + switch-all + status
│   └── hooks.ts                  # CHANGED: useLlmSettings / useUpdateLlmSettings
└── lib/api.ts                    # CHANGED: api.settings.getLlm/putLlm/llmModels

packages/shared/src/generated.ts  # REGEN via tygo
```

**Structure Decision**: Existing monorepo web-app layout (Go `apps/api`, React
`apps/dashboard`, shared TS `packages/shared`). New backend code lives in a new
`internal/llmsettings` package plus additions to `internal/llm`; frontend changes are
scoped to the existing `features/settings` module. Types flow Go→TS via tygo and DB→Go via
sqlc per Constitution III.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| External AI provider (Principle V) | User requires Cerebras free-tier models selectable at runtime | Env-only provider swap (no dashboard) rejected: spec requires a per-task dashboard toggle without restart. Kept local-first by defaulting to Ollama and gating Cerebras behind an operator-supplied key. |
| Runtime settings store + Router (vs startup env) | Per-task switch must apply without restart and persist | Baking model/provider at startup (today's approach) cannot satisfy FR-005 (no restart) or FR-004 (persist via UI). Router is the minimal seam: services keep the `Provider` interface unchanged. |
