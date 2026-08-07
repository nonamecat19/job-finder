# Implementation Plan: Split-Model Resume Generation

**Branch**: `035-split-model-generation` | **Date**: 2026-08-07 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/035-split-model-generation/spec.md`

## Summary

Resume generation is one LLM call doing four jobs. `selectAndTailor` writes the summary,
reorders skill groups, picks highlights and rephrases them in a single structured response, so
the whole document is priced at whatever model is good enough for the hardest of those jobs.

Split it into stages addressed by task key — `generation-analyze`, `generation-select`,
`generation-summary` — each backed by the cheapest model that does its job. Measured on
2026-08-07: **~$0.011 and ~20s per resume against ~$0.113 and ~60s** for a full premium run.

Three constraints shape the design. The application must never name a model (030-FR-004), so the
split is expressed as new task keys and new `*llm.Router` instances, not model identifiers in Go.
The economy model's characteristic failure is silent truncation, so a completeness verifier
weighted by vacancy relevance gates rendering. And the premium summary is immutable once written —
nothing downstream may reword it.

## Technical Context

**Language/Version**: Go 1.25 (`apps/api`), TypeScript 5.x + React 19 (`apps/dashboard`)

**Primary Dependencies**: chi (HTTP), sqlc + pgx (DB), goose (migrations), asynq (queues),
invopop/jsonschema (structured-output schemas), LiteLLM proxy (routing), rendercv (PDF)

**Storage**: PostgreSQL 16 + pgvector; `GeneratedDocument`, `ActivityRun` are the tables this
feature touches

**Testing**: `go test` (unit + integration against real Postgres/Redis via Docker Compose),
`vitest` (dashboard), existing generation benchmark fixture

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Web service (Go API) + React dashboard + configuration-owned LLM routing

**Performance Goals**: ≤ ⅕ of the pre-split cost per resume; ≤ ½ the pre-split median wall clock
(SC-001/SC-002). Baseline: $0.113 / 60s. Target: ~$0.011 / ~20s

**Constraints**: every stage chain terminates at local Ollama; no provider credential or model id
reachable from app or dashboard; per-stage application deadlines (the proxy's `request_timeout`
was observed unenforced — one call hung 830s); output caps sized per stage

**Scale/Scope**: single-user self-hosted deployment; ~4 LLM calls per resume today, 3 after this
change (cover letter leaves the default path)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Assessment |
|---|---|
| **I. No Auto-Apply** | Untouched. This feature changes how a document is produced, never how it is sent. No new outbound path. **PASS** |
| **II. Grounded Generation** | Strengthened. Summary gains independent grounding verification (FR-008), selection gains a completeness gate (FR-006), and FR-018 forbids weakening any existing rule. **PASS** |
| **III. Typed Contracts** | `TailoredSelection` / `TailoredSummary` are Go types with tygo-generated TS counterparts; the substitution marker reaches the dashboard through `packages/shared`, not a hand-written duplicate. **PASS** |
| **IV. Test Discipline** | Unit tests per stage in Go; verifier tests against truncated fixtures; dashboard marker in vitest. Touches `apps/api` + `apps/dashboard` + `packages/shared`, so `make test-lint` is required before done. **PASS** |
| **V. Local-First** | Each new task key gets its own chain terminating at the shared `local` Ollama deployment (FR-011). With no gateway configured, all three stages route to Ollama exactly as today. Credentials stay in the gateway container. **PASS** |

No violations. Complexity Tracking section omitted.

**Post-Phase-1 re-check**: design introduces three routers where one existed, and two types where
one existed. Both are direct consequences of the split rather than added abstraction — the
alternative (one router, model chosen per call site) would put model identity in application code
and break Principle V's routing contract. Still **PASS**.

## Project Structure

### Documentation (this feature)

```text
specs/035-split-model-generation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── contracts.md
├── checklists/
│   └── requirements.md
└── spec.md
```

### Source Code (repository root)

```text
apps/api/
├── cmd/server/
│   └── compose.go                     # + 3 routers; composeGeneration takes a router set
├── internal/generation/
│   ├── application/
│   │   ├── service.go                 # stage orchestration, escalation, cover-letter split
│   │   └── rendercv_llm.go            # analyze / select / summary prompt + call per stage
│   ├── domain/
│   │   ├── rendercv.go                # TailoredSelection, TailoredSummary, MergeTailored
│   │   ├── rendercv_completeness.go   # NEW: vacancy-weighted completeness verifier
│   │   └── rendercv_grounding.go      # summary-specific grounding checks
│   ├── interfaces/http/
│   │   └── documents.go               # + POST /documents/{id}/cover-letter
│   └── interfaces/worker/
│       └── handler.go                 # job-triggered path stops writing cover letters
├── internal/db/migrations/
│   └── 00038_document_stage_provenance.sql   # NEW
└── internal/platform/llm/
    └── domain/port.go                 # strictifySchema (already in tree)

apps/dashboard/src/features/tailor/
└── TailorPage.tsx                     # substitution marker; cover letter on demand

packages/shared/                       # regenerated DTOs (tygo)

gateway/config.yaml                    # 3 new task keys + chains + reasoning switches
```

**Structure Decision**: existing web-service layout (Go API + React dashboard + shared TS types).
This feature adds no new app or package; it extends the generation module's application and domain
layers, adds one migration, one endpoint, and three routing keys in gateway configuration.

## Phase 0: Research

See [research.md](./research.md). Nine decisions, all resolved — no NEEDS CLARIFICATION remains.
The load-bearing ones:

- **R1**: stage-to-model assignment (economy = gemini-2.5-flash-lite, premium = claude-sonnet-5),
  measured, with the reasoning switch each model actually honours.
- **R2**: reasoning control is a routing-config concern, not an app concern — the fix for the bug
  that made every run fail.
- **R4**: completeness is measured on *skill tokens* matched against vacancy analysis, not on
  group counts, because the clarified thresholds are vacancy-weighted.
- **R7**: the analysis→verifier coupling the clarify session surfaced, and how a thin analysis
  degrades safely instead of silently passing everything.

## Phase 1: Design

- [data-model.md](./data-model.md) — `TailoredSelection`, `TailoredSummary`, `StageOutcome`,
  `CompletenessReport`, the `GeneratedDocument` provenance columns, and migration 00038.
- [contracts/contracts.md](./contracts/contracts.md) — gateway task-key contract, the per-stage
  internal Go seam, the new cover-letter endpoint, and the DTO fields crossing to the dashboard.
- [quickstart.md](./quickstart.md) — runnable validation: prove each stage hit its own key, force a
  truncated selection, force a premium outage, and measure cost/latency against SC-001/SC-002.
