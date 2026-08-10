# Implementation Plan: Resume Generation Workspace

**Branch**: `042-resume-generation-workspace` | **Date**: 2026-08-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/042-resume-generation-workspace/spec.md`

## Summary

A dedicated `/generate` route replaces the one-shot tailor surface with a two-pane workspace:
the generated resume on the left as individually addressable items, the vacancy and controls on
the right. The AI's contribution to profile-sourced content becomes **a ranking of indices, not
text** — `RankedSelection` carries `[]int` per entry, so a reworded, merged or invented bullet
is unrepresentable rather than detectable. AI *suggestions* live in a separate response type and
a separate item origin, unselected by default and marked unverified wherever they appear. Export
renders exactly the selected items in the displayed order: no expand call, no `TrimHighlights`,
no third opportunity to reword.

This is the same design move 028 and 035 already made twice in this pipeline — delete the field
through which the violation would be expressed — applied to the last place free text still
reaches the profile-sourced group.

## Technical Context

**Language/Version**: Go 1.26.5 (`apps/api`); TypeScript 5 / React 19 (`apps/dashboard`)

**Primary Dependencies**: chi v5, sqlc, goose, pgx, asynq, LiteLLM gateway → Ollama
(`internal/llm` routers); React 19 + Vite 8 + TanStack Query 5 + dnd-kit 6 + Tailwind 4 +
react-router-dom 7; tygo for Go→TS DTO generation

**Storage**: PostgreSQL — three new tables (`generation_runs`, `generation_sections`,
`generation_items`) via goose migrations `00042`/`00043`. No pgvector, no Redis state: the run is
enqueued through the existing asynq `generate` queue.

**Testing**: `go test ./...` (unit + `dbtest` integration), `vitest` (dashboard),
`make test-integration` / `make test-e2e` against real Postgres/Redis. The 038 eval corpus gains
cases for the ranking contract (see `research.md` R7); the scorer set gains a
`ranking_violations` scorer delegating to the new domain verifier.

**Target Platform**: Linux server via Docker Compose (dev + prod)

**Project Type**: Web — Go API + React dashboard + shared TS type package

**Performance Goals**: SC-006 — a selection toggle repaints the preview with **zero** model calls
and under 1 s, which the design guarantees by construction: selection state is client state
persisted asynchronously, and the preview is derived from it locally. SC-002 — vacancy to
approved export under 3 min, dominated by the existing analyze/rank/summary/suggest stage budget
(90 s + 240 s + 120 s + 120 s ceilings, typically far less).

**Constraints**: FR-009 — profile-sourced item text is byte-identical to master, at every
grounding level. FR-018 — no post-selection model pass on the export path. FR-013 — zero
AI-suggested content in an export the user took no action on (SC-004). Constitution V — the whole
path must work with no gateway configured, i.e. suggestions must route to a task key whose chain
terminates at the local model.

**Scale/Scope**: one user, one profile, typically ≤10 experience entries × ≤20 master bullets and
≤10 skill groups. Item counts stay in the low hundreds per run — no pagination, no virtualisation.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | How this design satisfies it |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ PASS | The workspace produces a local draft and a PDF. No endpoint added here contacts an employer or submits anything. Export is a user-initiated render. |
| **II. Grounded Generation** | ✅ **STRENGTHENED** | Profile-sourced items become index references with no text field at all — strictly stronger than today's `HighlightRef{sourceIndex, rephrased}`, where a rewording still reaches the page under moderate/aggressive grounding. AI suggestions are a *separate* origin, off by default, marked unverified, never presented as the user's material (FR-016), and recorded per exported item (FR-023). Suggestions do not weaken II: II governs content presented as the user's experience, and a suggestion is presented as the opposite. |
| **III. Typed Contracts Across Boundaries** | ✅ PASS | Every new wire shape is a Go DTO in `internal/dto/generation_workspace.go` → `make tygo-generate` → `packages/shared/src/generated.ts`. No hand-written TS shape. DB access via sqlc from `internal/db/queries/generationrun.sql`. |
| **IV. Test Discipline Per Language** | ✅ PASS | Domain ranking/validation is pure Go with table tests; the workspace REST surface gets `dbtest`-backed integration tests; the workspace UI gets vitest tests including the "export with no user action contains zero suggestions" assertion. Change spans `apps/api`, `apps/dashboard` and `packages/shared`, so `make test-lint` is the merge gate. |
| **V. Local-First, Self-Hosted** | ✅ PASS | Suggestions reuse the existing `generation-select` task key rather than introducing a new gateway group (research R4), so no `gateway/config.yaml` change is required and the chain already terminates at local. With no gateway configured every stage routes to Ollama, unchanged. |

**Post-design re-check**: ✅ PASS, unchanged. Phase 1 introduced no third-party call, no new
hand-maintained type, and no code path that emits a document without an explicit user action.

Two facts found during research that the plan must state rather than assume — both are
documentation drift, not violations, and both are recorded in `research.md` R8:

1. `specs/domains/resume-generation.md` §4.1 documents a `/api/tailoring` draft/proposal REST
   surface. **It does not exist in the tree.** `internal/tailoring/` holds only proposal
   validators; there is no handler, no service, and nothing consumes
   `queue.GeneratePayload.TailoringDraftID`. Migration `00036` created the tables; they are unused.
2. `specs/domains/resume-generation.md` §7.1 documents a chromedp density-ladder page fitter at
   `internal/generation/singlepage`. **That package contains only `doc.go`.** The real render path
   is `RenderCvRenderer` (Typst) driven by `renderToPageTarget`.

Neither is reusable, so this plan builds the overflow report on the real render path (research R5)
and does not claim the fitter. Correcting the domain document is in scope for this feature's
ship-time fold-in, per the constitution's "exactly one copy of every binding rule".

## Project Structure

### Documentation (this feature)

```text
specs/042-resume-generation-workspace/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── rest-api.md      # HTTP surface: paths, bodies, status codes
│   └── llm-contracts.md # RankedSelection / SuggestionSet response types + validators
├── checklists/
│   └── requirements.md  # pre-existing
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/db/
│   ├── migrations/
│   │   ├── 00042_generation_runs.sql          # runs, sections, items
│   │   └── 00043_drop_tailored_drafts.sql     # retire the unwired 020 scaffolding
│   └── queries/generationrun.sql              # sqlc source
├── internal/dto/
│   └── generation_workspace.go                # tygo source for every new wire shape
└── internal/generation/                       # the feature module 042 extends
    ├── domain/
    │   ├── ranking.go            # RankedSelection, SuggestionSet, RankedItem
    │   ├── ranking_verify.go     # VerifyRanking: coverage, duplicates, range
    │   ├── assemble.go           # SelectionState + master → RendercvMaster (no LLM)
    │   └── overflow.go           # ranked drop candidates when the export is over budget
    ├── application/
    │   ├── workspace.go          # run lifecycle: start, per-section rerun, staleness
    │   ├── workspace_export.go   # render-once export path (no expand/condense)
    │   ├── rankcv_llm.go         # buildRankPrompt / buildSuggestPrompt
    │   └── evaldata/cases/…      # new corpus cases for the ranking contract
    ├── interfaces/http/
    │   └── generations.go        # the /v1/generations surface
    └── interfaces/worker/
        └── handler.go            # dispatch on GeneratePayload.GenerationRunID

apps/dashboard/src/
├── app/routes.tsx                             # + /generate
└── features/generate/
    ├── GenerateWorkspacePage.tsx              # two-pane shell
    ├── hooks.ts                               # TanStack Query hooks
    └── components/
        ├── SummaryBlock.tsx
        ├── WorkEntryBlock.tsx                 # ranked + suggested groups per entry
        ├── SkillsBlock.tsx
        ├── ItemRow.tsx                        # toggle, drag handle, origin badge
        ├── OriginBadge.tsx                    # "from your profile" / "AI · unverified"
        └── VacancyPane.tsx                    # vacancy, controls, rerun, export

packages/shared/src/generated.ts               # regenerated, never hand-edited
```

**Structure Decision**: 042 extends the existing `internal/generation/` feature module rather
than creating a new one. The ranking contract, its verifier and the prompt builders are the same
subject matter as `rendercv.go`/`rendercv_grounding.go`/`rendercv_llm.go` and share their
helpers; splitting them across two modules would force either duplication or a cross-feature
reach-in, both of which 027-FR-012 exists to prevent. The module already has `interfaces/http`
(`documents.go`) and `interfaces/worker`, so 027-FR-002 needs nothing new and adding the routes
costs one registration line in `cmd/server/compose.go` (027-SC-002).

On the dashboard, `features/generate/` is a new sibling of `features/tailor/`. The two coexist
through the transition, exactly as the spec's first assumption requires; `features/tailoring/`
(currently `export {}`) is deleted with the 020 scaffolding.

## Complexity Tracking

> No Constitution Check violations. This table records the two scope decisions that a reviewer
> would otherwise expect to see justified.

| Decision | Why | Simpler alternative rejected because |
|---|---|---|
| New tables rather than reusing `tailored_drafts` / `edit_proposals` | 042's unit is a *ranked item with an origin and a selected state*; 020's is an *accept/reject diff against a baseline*. The `edit_proposals.field_type` CHECK constraint and the `status` lifecycle encode the diff model directly. | Retrofitting would mean widening two CHECK constraints, adding four nullable columns and carrying a `field_type` vocabulary no 042 code reads — a schema describing two features and matching neither. The old tables are unused (no writer exists), so dropping them costs nothing. |
| Dropping `rephrased` from the profile-sourced contract entirely | FR-009 requires byte-identical text. A field that must always equal its source is a field with no legal non-empty value. | Keeping it and validating would preserve exactly the "check the model's free text" pattern §2a of the domain doc records as the failure mode this pipeline has already corrected twice. |
