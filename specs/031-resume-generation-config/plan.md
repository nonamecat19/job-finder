# Implementation Plan: Configurable Resume Generation Shape

**Branch**: `031-resume-generation-config` | **Date**: 2026-08-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/031-resume-generation-config/spec.md`

## Summary

Resume shape is currently hardcoded in two places: prompt literals in `generation/application/rendercv_llm.go` ("3-4 sentences", "TOP 8-10 highlights", "4-5 sentences", "TOP 5-6") and a fixed 2-page target in `renderResume` (`generation/application/service.go`). Projects pass through the pipeline verbatim with no selection or limiting.

This feature introduces a single persisted **shape config** (summary length, skills volume + enable, bullets per experience entry, target pages, projects enable + count + bullets per project), read at the start of every generation and threaded through three enforcement layers:

1. **Prompt-level** — approximate targets (summary length, bullets per role, skills volume) are injected into the tailor/expand/condense prompts instead of literals.
2. **Deterministic merge-level** — hard limits (section disable, project count, bullets per project, bullet upper cap) are applied in `MergeTailored`/post-merge so they cannot be missed by the model.
3. **Render-loop-level** — the page-fit loop targets the configured page count instead of `2`.

Projects gain a tailoring path (`TailoredProject{name, highlights}`) that selects and rephrases only within each project's own master bullets, with names/links/dates preserved verbatim by the merge, plus grounding coverage. The config is exposed through the existing settings surface (`/v1/settings/...`, `dto` + tygo + a dashboard card), following the `aifeature` package's shape exactly.

## Technical Context

**Language/Version**: Go 1.24 (`apps/api`), TypeScript 5 / React 19 (`apps/dashboard`)

**Primary Dependencies**: chi (HTTP routing), sqlc (typed DB access), goose (migrations), pgx, `platform/llm` structured-completion helpers (`llm.CompleteStructured` with jsonschema tags), rendercv + Typst (PDF render), TanStack Query + Tailwind (dashboard)

**Storage**: PostgreSQL. New singleton table `ResumeShapeSetting` (goose `00034`), read/written via sqlc-generated queries in `apps/api/internal/db/queries/resumeshapesetting.sql`.

**Testing**: `go test ./...` for `apps/api` (table-driven unit tests in `generation/domain`, handler tests, `dbtest`-backed integration), `vitest` for the dashboard. `make test-lint` gates the cross-app change.

**Target Platform**: Linux server, self-hosted Docker Compose

**Project Type**: Web service (Go API) + React dashboard, monorepo with `packages/shared` typed contract

**Performance Goals**: No new LLM round-trips on the default path. Projects are only sent to the LLM when a project limit or per-project bullet limit is configured; with defaults (unlimited) the projects prompt block is omitted entirely, so token usage and latency are unchanged from today.

**Constraints**: Defaults must be behaviour-equivalent to the current pipeline (FR-003). No fabrication (FR-017) — every enforcement path is either a truncation of existing content or a prompt instruction bounded by grounding checks. The page-fit loop keeps a bounded attempt count so generation cannot spin.

**Scale/Scope**: Single-user self-hosted deployment, one global config row. ~10 config values, 1 migration, 1 new Go package, ~6 touched Go files in `generation/`, 1 new dashboard card.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment | Verdict |
|-----------|------------|---------|
| **I. No Auto-Apply, Ever** | Feature only changes the shape of generated documents. No submission path is touched. | PASS |
| **II. Grounded Generation** | The new projects tailoring path is the only new LLM-generated content. Mitigations: project name/URL/dates are never sent back by the model (merge copies them from the deep-cloned master, same pattern as experience company/dates); highlights are constrained to each project's own master bullets by prompt; `VerifyRendercvGrounding` is extended to assert merged project names ⊆ master project names and (at strict level) project highlight tokens ⊆ that project's master token pool. No config value can cause invention — minima are satisfied by truncation only, never padding (FR-017). | PASS |
| **III. Typed Contracts Across Boundaries** | Config DTO lives in `internal/dto`, flows to TS via tygo into `packages/shared/src/generated.ts`; DB access is sqlc-generated. No hand-written duplicate types; `scripts/tygo-check.sh` enforces regeneration. | PASS |
| **IV. Test Discipline Per Language** | Go table-driven tests for shape defaults/validation/enforcement, merge, grounding and the page loop; handler tests for the new endpoints; vitest for the dashboard card; `make test-lint` before done. | PASS |
| **V. Local-First, Self-Hosted** | Config persists in the existing self-hosted Postgres; all generation continues to run through the existing local Ollama / LiteLLM routing. No new external dependency. | PASS |

**Result**: All gates pass. No entries required in Complexity Tracking.

**Post-Phase-1 re-check**: Design introduces one table, one package, one DTO, and one dashboard card — no new service boundary, no new external call, no new architectural layer. Grounding coverage grows rather than shrinks (projects were previously unverified because they were untouched; they are now both touched and verified). Gates still pass.

## Project Structure

### Documentation (this feature)

```text
specs/031-resume-generation-config/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── settings-resume-shape.md   # HTTP contract for the config endpoints
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
apps/api/
├── cmd/server/
│   └── compose.go                              # wire resumeshape service into generation + router
├── internal/
│   ├── db/
│   │   ├── migrations/00034_resume_shape_setting.sql   # NEW singleton table
│   │   ├── queries/resumeshapesetting.sql              # NEW sqlc queries
│   │   └── sqlcgen/                                    # regenerated
│   ├── dto/
│   │   └── settings.go                          # + ResumeShapeConfigDto
│   ├── resumeshape/                             # NEW package (mirrors internal/aifeature)
│   │   ├── service.go                           # load/cache/update/reset, validation
│   │   └── interfaces/http/resume_shape.go      # GET/PUT/DELETE /v1/settings/resume-shape
│   └── generation/
│       ├── domain/
│       │   ├── rendercv_shape.go                # NEW: ShapeConfig, defaults, validate, apply
│       │   ├── rendercv.go                      # + TailoredProject, MergeTailored project path
│       │   ├── rendercv_grounding.go            # + project grounding checks
│       │   └── rendercv_config.go               # + section removal keeps `_order` consistent
│       └── application/
│           ├── rendercv_llm.go                  # prompts parameterised by ShapeConfig
│           └── service.go                       # shape port, page-target loop, shortfall recording
└── (tests colocated as *_test.go beside each file)

apps/dashboard/src/
├── features/settings/
│   ├── ResumeShapeCard.tsx                      # NEW config card
│   ├── ResumeShapeCard.test.tsx                 # NEW vitest
│   ├── hooks.ts                                 # + useResumeShape / useUpdateResumeShape / useResetResumeShape
│   └── SettingsPage.tsx                         # mount the card
└── lib/
    ├── api.ts                                   # + settings.getResumeShape / putResumeShape / resetResumeShape
    └── queryKeys.ts                             # + resumeShape keys

packages/shared/src/generated.ts                 # regenerated by tygo
```

**Structure Decision**: Existing monorepo layout is reused unchanged. The config service is a new top-level `internal/resumeshape` package deliberately mirroring `internal/aifeature` (flat `service.go` + `interfaces/http/`), because it is settings CRUD rather than a domain with behaviour. The *shape value type* lives in `generation/domain` (not in `resumeshape`) so the generation pipeline depends only on its own domain package; `resumeshape` imports `generation/domain`, giving a one-way dependency and no cycle.

## Complexity Tracking

No constitution violations. Section intentionally empty.
