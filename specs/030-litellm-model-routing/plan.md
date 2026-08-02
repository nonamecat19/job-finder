# Implementation Plan: Gateway-Owned Model Routing

**Branch**: `030-litellm-model-routing` | **Date**: 2026-07-31 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/030-litellm-model-routing/spec.md`

## Summary

Delete the per-task provider/model selection feature end to end (dashboard tile, HTTP endpoints, DB table, Cerebras adapter, snapshot-holder routing machinery) and let the LiteLLM proxy own every model decision. The Go backend keeps exactly one routing input — the task key (`match`, `generation`, `rephrase`, `ghost`, `default`) — which it sends as the OpenAI `model` field. `gateway/config.yaml` maps each task key to an ordered failover chain: free-tier hosted providers (Cerebras → Groq → Cohere) first, then OpenRouter, terminating on the Ollama deployment so the chain never dead-ends. When `GATEWAY_URL` is unset or the proxy is unreachable, the backend talks to Ollama directly, preserving Constitution Principle V.

`llm.Router` survives as a name but loses its dynamic snapshot: it becomes a static task-bound wrapper picking gateway-if-configured-else-Ollama at construction, which keeps every call site (`compose.go`, worker handlers, `queue.ClassResolver`) unchanged in shape.

## Technical Context

**Language/Version**: Go 1.24 (`apps/api`), TypeScript 5 / React 19 (`apps/dashboard`), YAML config (`gateway/config.yaml`)

**Primary Dependencies**: chi router, sqlc + goose (Postgres), asynq (queues), LiteLLM proxy (`ghcr.io/berriai/litellm:main-stable`), Ollama, TanStack Query, tygo (Go→TS types)

**Storage**: Postgres — this feature *drops* the `"LlmTaskSetting"` table and adds no new tables. No new state is persisted; routing state lives in `gateway/config.yaml` and environment variables only.

**Testing**: `go test ./...` (unit + `-tags=integration` against Docker Postgres/Redis), `vitest` for the dashboard, `make test-lint` for the cross-app gate

**Target Platform**: Linux, self-hosted Docker Compose single-operator deployment

**Project Type**: Web application — Go API + React dashboard + sidecar services (Postgres, Redis, Ollama, LiteLLM)

**Performance Goals**: No added latency on the happy path (one proxy hop, unchanged); a failed-provider failover must stay inside the task's existing timeout budget (gateway HTTP client timeout is 120 s)

**Constraints**: Free-tier providers before OpenRouter (FR-006); local model terminates every chain (FR-008); backend must never learn which upstream served a request except for logging (FR-012/FR-013); no credentials in the DB or in any API response (FR-010); missing keys must not crash startup (FR-011)

**Scale/Scope**: 5 task keys, 4 hosted providers + 1 local, ~13 Go files touched (mostly deletions), ~7 dashboard/shared files, 1 migration, 1 gateway config rewrite

## Constitution Check

*GATE: evaluated before Phase 0 and re-checked after Phase 1.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. No Auto-Apply | ✅ Pass | Feature touches model routing only; no submission path involved. |
| II. Grounded Generation | ✅ Pass | Prompts, schemas and grounding levels are untouched. A provider swap does not relax grounding; `CompleteStructured` validation still runs on every response. |
| III. Typed Contracts | ✅ Pass | DTO removals flow through `sqlc generate` and `tygo` regeneration; `packages/shared/src/index.ts` hand-written DTOs deleted in the same change. No hand-edited generated files. |
| IV. Test Discipline | ✅ Pass | Go unit tests for the new static `Router`, gateway served-model logging, and config; vitest for the shrunken Settings page; `make test-lint` before done. Deleted features take their tests with them. |
| V. Local-First, Self-Hosted | ✅ Pass with note | The system stays fully operational with zero external calls: `GATEWAY_URL` empty → direct Ollama (FR-009), and the in-proxy chain terminates on the Ollama deployment (FR-008). The user explicitly asked for free hosted tiers to be *preferred*; that is a preference order, not a hard dependency, and no paid API is required for any core flow. OpenRouter (potentially paid) is reached only after every free tier fails and can be removed from the chain by editing one file. |

**Post-Phase-1 re-check**: unchanged — no design artifact introduced a new persisted entity, a new external hard dependency, or a hand-maintained duplicate type. Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/030-litellm-model-routing/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── gateway-config.md
│   ├── removed-surface.md
│   └── task-router.md
├── checklists/
│   └── requirements.md
└── tasks.md             # /speckit-tasks output — NOT created here
```

### Source Code (repository root)

```text
gateway/
└── config.yaml                                   # REWRITTEN: task groups + ordered fallback chains

docker-compose.yml                                # litellm env: + GROQ/COHERE/CEREBRAS keys, OLLAMA_URL/KEY
.env.example                                      # + GROQ_API_KEY, COHERE_API_KEY; − dashboard-model prose

apps/api/
├── cmd/server/
│   ├── compose.go                                # composeLLM simplified; llmsettings wiring removed
│   └── servers.go                                # LlmSettings.Mount removed
├── internal/
│   ├── config/{config.go,defaults.go}            # Cerebras* removed; gateway vars documented
│   ├── db/
│   │   ├── migrations/00033_drop_llm_task_setting.sql   # NEW
│   │   ├── queries/llmsetting.sql                # DELETED
│   │   └── sqlcgen/                              # regenerated (llmsetting.sql.go, models.go)
│   ├── dto/settings.go                           # Llm*/CerebrasModelDto removed
│   ├── llmsettings/                              # DELETED (whole bounded context)
│   └── platform/llm/
│       ├── llm.go                                # facade: drop snapshot/cerebras exports
│       ├── application/router.go                 # static task Router (no SnapshotHolder)
│       └── infrastructure/
│           ├── cerebras/                         # DELETED
│           ├── gateway/gateway.go                # echo served model, structured logging
│           └── shared/errors.go                  # kept; breaker dropped if unreferenced
└── ...

apps/dashboard/src/
├── features/settings/
│   ├── LlmSettingsCard.tsx, LlmSettingsCard.test.tsx   # DELETED
│   ├── SettingsPage.tsx, SettingsPage.test.tsx         # "AI models" tile removed
│   └── hooks.ts                                        # useLlmSettings/useLlmModels/useUpdate* removed
├── features/status/StatusPage.tsx                      # provider-specific copy genericised
└── lib/{api.ts,queryKeys.ts}                           # settings.getLlm/putLlm/llmModels + keys removed

packages/shared/src/{index.ts,generated.ts}       # Llm*/Cerebras* DTOs removed + regenerated
```

**Structure Decision**: Existing web-app layout (`apps/api` Go hexagonal contexts + `apps/dashboard` React + `packages/shared` types + `gateway/` proxy config). This feature adds no new module; it deletes one bounded context (`internal/llmsettings`), one infrastructure adapter (`infrastructure/cerebras`), and one dashboard feature card, and moves the deleted behaviour into `gateway/config.yaml`.

## Phase 0 — Research

See [research.md](./research.md). Resolved unknowns: LiteLLM ordered-failover mechanics (`litellm_settings.fallbacks` between model groups, not multi-deployment groups), missing-credential behaviour, JSON-mode capability per provider vs `drop_params`, served-model observability, admission-gate class derivation without per-task settings, and the goose-down strategy for a table drop.

## Phase 1 — Design & Contracts

- [data-model.md](./data-model.md) — task keys, routing chain, provider credentials, and the deleted entity + migration shape.
- [contracts/gateway-config.md](./contracts/gateway-config.md) — the config the proxy must satisfy: group names, chain order, timeouts, cooldowns.
- [contracts/task-router.md](./contracts/task-router.md) — the internal Go contract replacing the snapshot Router, including `ProviderClass` semantics for the admission gate.
- [contracts/removed-surface.md](./contracts/removed-surface.md) — the exact HTTP, TS, and DB surface that must be gone, usable as a deletion checklist.
- [quickstart.md](./quickstart.md) — runnable validation: fresh boot, free-tier hit, forced failover, gateway-down fallback, and settings-page regression checks.

## Complexity Tracking

No constitution violations to justify. Net complexity decreases: one bounded context, one adapter, one table, three endpoints, and one dashboard card are removed; the only addition is declarative YAML.
