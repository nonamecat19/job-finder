# Implementation Plan: Strict Resume Structure Preservation During AI Tailoring

**Branch**: `028-resume-structure-preservation` | **Date**: 2026-07-31 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/028-resume-structure-preservation/spec.md`

## Summary

Tighten the AI resume tailoring pipeline (feature 020) with three non-negotiable structural-integrity invariants enforced automatically (not user-accept/reject): (1) resume block sequence is immutable — the AI may not add, remove, rename, or reorder blocks; (2) experience (job) entries keep the master's authored order — the AI may not reorder or drop jobs; (3) total years of experience is locked — the AI may not alter an explicit figure, alter experience dates, or assert a contradicting years figure in generated text. Violating proposals are dropped with a structural-integrity reason and surfaced in the existing diff/review surface with a non-technical label, never offered as accept/reject.

The implementation lands in the existing `apps/api/internal/generation` package (Go) and the `generation` domain — it removes AI capabilities (`SectionsToDrop`, `ExperienceOrder`, `Drop`) rather than adding new surface area, and adds a post-merge structural-integrity verifier alongside the existing grounding verifier.

## Technical Context

**Language/Version**: Go 1.24 (apps/api), TypeScript 5.x (apps/dashboard, packages/shared)

**Primary Dependencies**: existing `internal/generation` (tailoring pipeline), `internal/generation/domain` (RendercvMaster, MergeTailored, VerifyRendercvGrounding), `internal/dto` (Resume/Section/Entry), `packages/shared` (TS DTOs via tygo)

**Storage**: no schema change — the structural invariants are enforced in the tailoring/merge/grounding layer on the existing `RendercvMaster`/`tailored_drafts.baseline` jsonb. No new tables, no migration.

**Testing**: `go test` for the Go invariants (unit tests in `internal/generation/domain` and `application`); `vitest` for any dashboard diff-surface label changes.

**Target Platform**: Linux server (existing Docker Compose stack); local-first Ollama per constitution Principle V.

**Project Type**: web service (Go API) + React/Vite dashboard, monorepo.

**Performance Goals**: structural-integrity checks are pure in-memory map comparisons on already-parsed `RendercvMaster`; negligible cost (<1ms), no new LLM call. Tailoring latency budget unchanged from feature 020 (<60s typical).

**Constraints**: invariants enforced on every tailoring run including re-runs; no partial application of violating proposals; dropped structural proposals recorded with an auditable reason; no new external API calls (constitution Principle V).

**Scale/Scope**: ~1 domain file + ~1 verifier + prompt edits + 2 DTO field changes + small dashboard label wiring. No new endpoint, no new table, no migration.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. No Auto-Apply, Ever | ✅ Pass | Feature only constrains AI generation of resume content; it never submits, contacts employers, or acts on a listing. No new action path introduced. |
| II. Grounded Generation | ✅ Strengthens | The three invariants (block sequence, job order, total experience years) are direct extensions of Grounded Generation — they prevent the AI from misrepresenting structural facts and career trajectory. FR-007 explicitly suppresses inflated years claims as grounding violations. |
| III. Typed Contracts Across Service Boundaries | ✅ Pass | Two DTO field changes (`TailoredSections.SectionsToDrop`, `ExperienceOrder`, `Drop`) are removed from the Go struct and regenerated via tygo into `packages/shared`. `index.ts` hand-mirror updated per AGENTS.md convention. No hand-maintained duplicate types introduced. |
| IV. Test Discipline Per Language | ✅ Pass | New invariants covered by `go test` unit tests (domain + application); `make test-lint` is the merge gate. No new cross-service behavior; integration/e2e not required for pure in-memory invariants. |
| V. Local-First, Self-Hosted | ✅ Pass | No new external API; structural checks are pure Go. Tailoring continues to run against local Ollama. |

No violations. No Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/028-resume-structure-preservation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── structural-invariants.md   # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
apps/api/internal/generation/
├── application/
│   ├── rendercv_llm.go        # EDIT: buildSelectPrompt — drop reorder/drop/section-drop instructions
│   └── service.go             # EDIT: tailorRendercvResume — call VerifyStructureIntegrity
├── domain/
│   ├── rendercv.go            # EDIT: remove SectionsToDrop/ExperienceOrder/Drop from TailoredSections; MergeTailored — no reorder/drop
│   ├── rendercv_grounding.go  # EDIT (extend) or new rendercv_structure.go — VerifyStructureIntegrity + total-experience-years suppression
│   └── rendercv_test.go       # EDIT: add structure-integrity + years-invariant tests
apps/api/internal/dto/
└── (no change — Resume/Section/Entry unchanged; the constraint is on the tailoring payload, not the resume DTO)
packages/shared/src/
├── generated.ts               # REGENERATE via tygo after TailoredSections change
└── index.ts                   # EDIT: hand-mirror TailoredSections field removal (per AGENTS.md)
apps/dashboard/                 # small: surface dropped structural proposals with non-technical labels in existing diff view
```

**Structure Decision**: The feature is a pure constraint on the existing `apps/api/internal/generation` tailoring pipeline. No new package, no new endpoint, no new table — it removes AI capabilities (`SectionsToDrop`, `ExperienceOrder`, `Drop`) and adds a `VerifyStructureIntegrity` verifier alongside the existing `VerifyRendercvGrounding`. Dashboard change is a label-only edit to the existing diff/review surface.

## Complexity Tracking

> No Constitution Check violations — section left empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |