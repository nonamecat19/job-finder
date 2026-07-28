# Implementation Plan: Constrained AI Resume Tailoring with Single-Page PDF Output

**Branch**: `020-ai-resume-tailoring` | **Date**: 2026-07-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/020-ai-resume-tailoring/spec.md`

## Summary

Add a constrained, review-gated AI resume tailoring flow with single-page PDF output. The AI is only permitted to propose edits to three allow-listed surfaces — the professional summary, the per-company work-experience description bullets (highlights), and the skills section (skill groups addable/removable whole, individual skills addable/removable) — derived from and traceable to the user's master resume plus a target job posting. Every proposal is presented to the user for field-level accept/reject before the tailored resume is finalized; rejected fields revert to the baseline. The final resume exports as exactly one text-based PDF page; when content cannot fit even at minimum-density bounds, export is blocked with actionable feedback. Re-runs compare against the current baseline (master + previously accepted edits), not the original master, so accepted edits are never re-surfaced.

Net-new infrastructure: (a) a persistent `tailored_draft` + `edit_proposal` store so the review/diff state survives between sessions; (b) an extension to the RenderCV LLM output schema (`TailoredSections`) to express skill-group add/remove and to emit per-field proposals keyed to the baseline; (c) a single-page PDF renderer-fitter built on the existing chromedp pipeline with a measure-then-fit density loop and an HTML resume template over the `dto.Resume` shape; (d) a dashboard review UI under `features/job-detail` for proposal diff/accept/reject and single-page export. All AI work reuses the existing `generate` asynq worker, LLM `Router`, and activity recorder; no new worker tier is introduced.

## Technical Context

**Language/Version**: Go 1.24 (backend), TypeScript 5.x (dashboard, Vite + React 19).

**Primary Dependencies**:
- Backend: `chi` (router), `sqlc` (typed DB), `asynq` (workers), `pgvector` (embeddings — unchanged), `chromedp` (headless Chromium — already a dep, reused for the single-page fitter), `ollama` + `cerebras` LLM providers (existing `internal/llm`). `rendercv` CLI (binary on PATH) — kept for "export as-is" but NOT used for the single-page tailored export.
- Dashboard: `@tanstack/react-query`, `@dnd-kit`, `tailwind`. No new runtime deps.

**Storage**: Postgres (new tables: `tailored_drafts`, `edit_proposals` — see `data-model.md`). MinIO (existing, unchanged) for produced PDFs. Redis/asynq (existing `generate` queue, reused).

**Testing**: `go test` (unit), `//go:build integration` (real Postgres/Redis via `make test-integration`, `dbtest.LockSharedDB` serialization), `vitest` (dashboard unit, `*.test.tsx`), Playwright (`make test-e2e`). Live LLM/rendercv tests gated by naming, excluded from default suite.

**Target Platform**: Linux server (Docker Compose dev, prod unchanged); dashboard is a SPA.

**Project Type**: Web service (Go API + React SPA).

**Performance Goals**: < 60s wall-clock for a single tailoring run on the local Ollama model with a typical one-page resume (SC-007). Indeterminate progress at 30s. Non-blocking: user can navigate the dashboard mid-run. Existing `generate` queue's `max_duration` already covers this (feature 019), bumped from 120s default if needed.

**Constraints**: Single-page PDF is non-negotiable (SC-002/006). Strict local/self-hosted model only (constitution Principle V). No auto-apply / employer contact (Principle I, FR-009). ATS-readable text PDF (no raster) — chromedp `PrintToPDF` produces text PDFs natively, satisfies FR-007.

**Scale/Scope**: Single user, one active tailoring draft per `(profile, job)` pair at a time; modest scale. Drafts/proposals retained per spec (no retention SLA stated — assume until user deletes profile or draft).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I — No Auto-Apply (NON-NEGOTIABLE) | ✅ Pass | FR-009 explicitly forbids the tailoring run from submitting applications or contacting employers; the flow produces only a local draft + PDF. No new outbound side-effects are introduced. |
| II — Grounded Generation | ✅ Pass | FR-003 + SC-005 + traceability pointer on every proposal; existing `verifyRendercvGrounding` reused and tightened to the allow-list; any AI edit lacking a traceable source (master field or job-posting signal) is suppressed. |
| III — Typed Contracts | ✅ Pass (with effort) | New `EditProposal` / `TailoredDraft` DTOs added to `apps/api/internal/dto/`, mirrored in `packages/shared/src/index.ts`, regenerated via `tygo generate`. `make sqlc-generate`/`make tygo-check` gates enforce. |
| IV — Test Discipline Per Language | ✅ Pass | Go unit tests in `internal/tailoring`/`internal/generation`; `//go:build integration` suite for the draft/persistence + grounding + single-page fitter against real Postgres + chromedp; vitest for dashboard review UI; `make test-lint` before merge. |
| V — Local-First | ✅ Pass | Tailoring routes through the `generation` `Router` which resolves to the local Ollama model by default (FR-008 of 019); Cerebras is only used when the operator has configured it for the `generation` task — that's a deployment decision, not a new hard dependency, and the local path remains the only MUST-work flow. No new external paid-API calls. |

No violations. No Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/020-ai-resume-tailoring/
├── plan.md              # This file
├── research.md          # Phase 0 output — resolved unknowns, design rationale
├── data-model.md        # Phase 1 output — new entities & schema
├── quickstart.md        # Phase 1 output — end-to-end validation guide
├── contracts/
│   ├── tailoring-api.md      # REST contract: proposal lifecycle
│   └── single-page-pdf.md   # Renderer contract: density-fit contract
└── checklists/
    └── requirements.md  # (existing) spec quality checklist
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── db/
│   │   ├── migrations/00030_tailoring_drafts.sql   # new tables
│   │   └── queries/tailoring.sql                    # sqlc queries
│   ├── dto/
│   │   └── tailoring.go                              # EditProposal/TailoredDraft DTOs
│   ├── tailoring/                                    # NEW package
│   │   ├── service.go        # propose/accept/reject/export, baseline resolution
│   │   ├── proposals.go       # diff generator baseline vs LLM payload
│   │   ├── service_test.go
│   │   └── integration_test.go
│   ├── generation/
│   │   ├── rendercv.go            # extend TailoredSections for group add/remove + per-field
│   │   ├── rendercv_proposals.go  # NEW: payload → []EditProposal generator
│   │   ├── singlepage/
│   │   │   ├── fitter.go          # NEW: chromedp measure+fit loop, bounded density
│   │   │   ├── template.go        # NEW: dto.Resume → HTML (no Theme, ATS-clean)
│   │   │   ├── fitter_test.go
│   │   │   └── integration_test.go
│   │   └── service.go             # wire new entry, keep RenderCvRenderer for as-is exports
│   ├── httpapi/
│   │   └── tailoring.go           # NEW: Mount-bearing handler
│   └── queue/
│       └── queue.go                # (no new task type — reuses TypeGenerate)
├── cmd/server/
│   ├── compose.go                  # +app.Tailoring
│   ├── compose_features.go         # +composeTailoring(...), passes generate worker
│   └── servers.go                  # +app.Tailoring.Mount to NewRouter variadic list

apps/dashboard/
└── src/features/tailoring/        # NEW
    ├── TailoringPanel.tsx
    ├── ProposalReview.tsx          # diff per field, accept/reject radio
    ├── ExportSinglePage.tsx
    ├── hooks.ts                    # tanstack-query mutations/queries
    ├── api.ts
    └── *.test.tsx

packages/shared/src/index.ts        # +EditProposalDto, TailoredDraftDto, etc.
```

**Structure Decision**: Single-repo monorepo (existing `apps/api` + `apps/dashboard` + `packages/shared`) — no new top-level apps. New Go package `internal/tailoring` owns proposal lifecycle & baseline resolution; new subpackage `internal/generation/singlepage` owns the single-page PDF fitter; the existing `internal/generation` package keeps its current responsibilities and gains a thin entry that returns proposals instead of merging. Dashboard gets a new `features/tailoring` module matched to the existing `features/job-detail` and `features/profile` modules in style. The reason for `internal/tailoring` rather than folding into `internal/generation`: the proposal/accept/reject/baseline logic is a distinct concern from the LLM prompt/merge logic, keeps `internal/generation` focused on "produce the AI output payload," and makes the single-page renderer a pure post-accept concern.

## Complexity Tracking

> No Constitution Check violations to justify — left empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|---------------------------------------|
| (none)    |            |                                       |