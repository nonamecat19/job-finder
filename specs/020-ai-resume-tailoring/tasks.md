---

description: "Task list for feature 020: Constrained AI Resume Tailoring with Single-Page PDF Output"
---

# Tasks: Constrained AI Resume Tailoring with Single-Page PDF Output

**Input**: Design documents from `/specs/020-ai-resume-tailoring/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/tailoring-api.md, contracts/single-page-pdf.md, quickstart.md

**Tests**: Included — the spec mandates integration tests per constitution Principle IV, and `quickstart.md` codifies them as `//go:build integration` suites plus dashboard vitest.

**Organization**: Tasks grouped by user story. US1 (P1) covers the core tailoring proposal loop. US2 (P2) covers single-page PDF export. US3 (P3) is a refinement of US1's skill-group flows and mostly tested via US1's infrastructure — its phase is lightweight.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Repos uses `apps/api/...` (Go), `apps/dashboard/...` (TS), `packages/shared/...` (TS) — no separate `src/`/`tests/` split; tests are co-located as `*_test.go` / `*.test.tsx`.

## Path Conventions (this repo)

- Go unit tests: `*_test.go` co-located in the package under test (e.g., `apps/api/internal/tailoring/service_test.go`).
- Go integration tests: `*_integration_test.go` co-located, guarded by `//go:build integration` build tag; run via `make test-integration` against real Postgres+Redis+chromedp via Docker Compose.
- Dashboard tests: `*.test.tsx` co-located in `apps/dashboard/src/features/tailoring/`.

---

## Phase 1: Setup (New Package Skeleton)

**Purpose**: Create the new Go packages, dashboard feature dir, and contracts dir laid out in plan.md so all subsequent tasks have a place to land.

- [ ] T001 [P] Create Go package `apps/api/internal/tailoring/` with empty `doc.go` (package comment summarizing purpose: proposal lifecycle, baseline resolution, single-page export trigger — references spec 020)
- [ ] T002 [P] Create Go subpackage `apps/api/internal/generation/singlepage/` with empty `doc.go` (package comment: chromedp-based ATS-clean single-page PDF fitter per contracts/single-page-pdf.md)
- [ ] T003 [P] Create dashboard feature dir `apps/dashboard/src/features/tailoring/` with empty `index.ts` (re-export surface for `TailoringPanel`, `ProposalReview`, `ExportSinglePage`, hooks)
- [ ] T004 [P] Create contracts dir `specs/020-ai-resume-tailoring/contracts/` — already populated by `/speckit.plan`; verify `tailoring-api.md` and `single-page-pdf.md` are present (no-op if so)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared DTOs, persisted state, queue payload extension, and tygo/sqlc scaffolding needed by every user story.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T005 [P] Define Go DTOs for `EditProposalDto`, `TraceabilityDto`, `TailoredDraftDto`, `BaselineSummaryDto`, `ExportBlockDto`, `TailorResumeRequestDto`, `AdhocVacancyDto`, `ExportPdfRequestDto` in `apps/api/internal/dto/tailoring.go` with `json` tags matching `data-model.md` DTO block exactly
- [ ] T006 Mirror the new DTOs in `packages/shared/src/index.ts` field-for-field (per AGENTS.md convention — `index.ts` is hand-maintained, NOT imported from `generated.ts`)
- [ ] T007 Run `tygo generate` (from `apps/api`) to regenerate `packages/shared/src/generated.ts` from the Go DTOs; run `make tygo-check` and verify both halves agree
- [ ] T008 [P] Write migration `apps/api/internal/db/migrations/00030_tailoring_drafts.sql` (verify next free integer ≥ `00023`, never reuse) creating `tailored_drafts` and `edit_proposals` tables per `data-model.md` schema (columns, FKs, `state`/`status` CHECK constraints, `unique(profile_id, coalesce(job_id, ...))`, `edit_proposals_draft_status_idx`, `updated_at` trigger)
- [ ] T009 Run `make sqlc-generate` to regenerate `apps/api/internal/db/sqlcgen/` from the new `apps/api/internal/db/queries/tailoring.sql` queries (`CreateDraft`, `GetDraft`, `GetDraftForUpdate`, `ListDraftsByProfileJob`, `AbandonDraft`, `SetDraftState`, `UpdateDraftBaseline`, `SetDraftExport`, `SetDraftExportStatus`, `CreateProposals` batch, `ListProposalsByDraft`, `ListPendingProposals`, `SetProposalStatus`); run `make sqlc-check` gate
- [ ] T010 [P] Write `apps/api/internal/db/queries/tailoring.sql` with the 13 queries above (pointer parameter for `baseline jsonb`, `vacancy_*` nullable, `coalesce` in the unique lookup; `SetProposalStatus` accepts `accepted_at`/`rejected_at` mutually-exclusive)
- [ ] T011 [P] Extend `apps/api/internal/queue/queue.go` `GeneratePayload` struct with one new `TailoringDraftID *uuid.UUID` field (json `"tailoringDraftId,omitempty"`); existing callers leave it nil and the merged-resume path is unchanged (research R5/contract backward-compat note)
- [ ] T012 [P] Add unit test `apps/api/internal/dto/tailoring_test.go` covering JSON round-trip for each new DTO (marshal → unmarshal → deep-equal) — guards the field-for-field mirror with `packages/shared`

**Checkpoint**: Foundation ready. `make test-go` clean. DTOs shared across the wire, DB schema live, queries typed, queue payload extended. User story implementation can begin.

---

## Phase 3: User Story 1 — Tailor Resume for a Specific Job Listing (Priority: P1) 🎯 MVP

**Goal**: AI tailoring run produces a set of field-level `EditProposal` rows restricted to the allow-list (summary, per-company highlights, skills/skill-groups), derived from the master resume and job posting, presented to the user via a review UI for accept/reject. Re-runs compare against the current baseline.

**Independent Test**: `POST /api/tailoring` with a profile+job → poll to `state="review"` → assert every `proposal.fieldType` ∈ `{summary, experience_highlights, skill_change, skill_group_add, skill_group_remove}` (SC-001) and no non-allow-list field changed vs master (FR-001/002). Accept and reject proposals; each mutation updates `tailored_drafts.baseline` and the proposal's own `status` atomically (FR-004/005/006). Re-run (`POST /rerun`) creates a sibling draft seeded from the current baseline; accepted edits do NOT re-appear as pending (FR-010, clarification Q4).

### Tests for User Story 1

> Constitution Principle IV mandates integration tests against real Postgres/Redis; tests written first per TDD where feasible.

- [ ] T013 [P] [US1] Write unit test `apps/api/internal/generation/rendercv_proposals_test.go` covering: (a) summary diff produces one `summary` proposal, (b) per-company highlights LCS-diff produces one `experience_highlights` proposal per changed bullet, (c) `SkillGroupsToAdd` → one `skill_group_add` proposal each, (d) `SkillGroupsToRemove` → one `skill_group_remove` proposal each, (e) `SkillChanges` → one `skill_change` proposal per token add/remove, (f) payload that attempts to alter a job title produces zero proposals for that field (allow-list enforcement at the generator)
- [ ] T014 [P] [US1] Write unit test `apps/api/internal/tailoring/service_test.go` covering: `Accept` mutates `baseline` and sets `proposal.status="accepted"` in the same tx; `Reject` leaves `baseline` unchanged and sets `status="rejected"`; re-run seeds new draft baseline from prior draft's current baseline (no ghost re-proposals); finalize on a draft with pending proposals returns an error
- [ ] T015 [P] [US1] Write integration test `apps/api/internal/tailoring/integration_test.go` with `//go:build integration` — `TestTailoring_AllowListEnforced` (Scenario 1 from `quickstart.md`): real Postgres, real `dbtest.LockSharedDB` serialization, a fixture profile and job, run the tailoring service end-to-end, assert allow-list compliance and accept/reject baseline mutation
- [ ] T016 [P] [US1] Write dashboard unit test `apps/dashboard/src/features/tailoring/ProposalReview.test.tsx` — ProposalReview renders proposals grouped by `fieldType`, accept/reject radios dispatch mutations, dropped proposals are not surfaced
- [ ] T017 [P] [US1] Write dashboard unit test `apps/dashboard/src/features/tailoring/hooks.test.tsx` — TanStack Query hooks call the correct endpoints with the correct bodies (mock fetch)

### Implementation for User Story 1

- [ ] T018 [US1] Extend `apps/api/internal/generation/rendercv.go` `TailoredSections` struct (research R3): add `Skills.SkillGroupsToAdd []TailoredSkillGroupAdd{Label, Details}`, `Skills.SkillGroupsToRemove []string`, `Skills.SkillChanges []SkillChange{GroupLabel, AddTokens, RemoveTokens, ReplaceDetails *string}` — keep existing fields backward-compatible so `GenerateAdHoc` (merged-resume path) is unaffected
- [ ] T019 [US1] Extend `apps/api/internal/generation/rendercv.go` `buildSelectPrompt` to enumerate the new skill-group add/remove instructions to the LLM, including the `masterSkillTokens` allow-list (already computed at `rendercv.go:362-371`) so the model knows which tokens it may use — never invents new skill strings
- [ ] T020 [US1] Extend `apps/api/internal/generation/grounding.go` and `rendercv_grounding.go` (`verifyRendercvGrounding`) to enforce the new tighter rules (research R3): reject `SkillGroupsToAdd` whose tokens are not all in `masterSkillTokens`; reject `SkillGroupsToRemove` labels not in master; reject `SkillChanges.AddTokens` not in `masterSkillTokens`; reject per-company `Highlights` whose LCS-matched set doesn't cover the proposed bullets (research R8 — drop novel bullets before they reach the proposal generator)
- [ ] T021 [US1] Write `apps/api/internal/generation/rendercv_proposals.go` — `PayloadToProposals(baseline RendercvMaster, payload TailoredSections, jobPosting) []EditProposal` (research R1): deterministic Go diff, one proposal per changed summary / per changed company-bullet (LCS over baseline highlights) / per skill-group add / per skill-group remove / per skill add/remove per existing group; attach `TraceabilityDto{source, path}` to each; suppress grounding-violating bullets with `status="dropped"` and `dropped_reason="grounding_violation"` (never surfaced)
- [ ] T022 [US1] Write `apps/api/internal/tailoring/proposals.go` — per-field validators (one per `field_type`) that enforce invariants on `before_value`/`after_value` shape (e.g., `skill_group_add.before_value == ""`, `skill_group_remove.after_value == ""`); single dispatch function used by the service layer
- [ ] T023 [US1] Write `apps/api/internal/tailoring/service.go`:
  - `RunProposals(ctx, draftID)` — called by the extended `generation.Handler.ProcessTask` when `GeneratePayload.TailoringDraftID != nil`; loads draft, fetches master baseline, invokes `analyzeVacancy` + `selectAndTailor` (existing) with the new `buildSelectPrompt`, calls `verifyRendercvGrounding`, calls `PayloadToProposals`, batch-inserts proposals (status `pending` or `dropped`), transitions draft `state` to `review`; on LLM unreachable/timeout/malformed per FR-014 transitions state to error and preserves baseline unchanged (no partial proposals persisted), no silent auto-retry
  - `Accept(ctx, draftID, proposalID)` — `GetDraftForUpdate`, validate proposal is `pending`/`draft.state="review"`, run per-field validator, mutate `baseline` per the field-specific merge (summary overwrite / highlights replace-at-index / skill group append / skill group drop / skill token add-or-remove within group), `SetProposalStatus("accepted", now)`, commit tx
  - `Reject(ctx, draftID, proposalID)` — `GetDraftForUpdate`, validate `pending`/`review`, `SetProposalStatus("rejected", now)`, leave `baseline` untouched
  - `Finalize(ctx, draftID)` — assert zero `pending` proposals, `SetDraftState("finalized")`
  - `Rerun(ctx, draftID)` — read current `baseline`, abandon prior draft as `finalized` (do NOT delete — audit), `CreateDraft` new row with `parent_draft_id=prior`, seed `baseline` from prior's current `baseline`, enqueue new `generate` task carrying the new draft id (research R7)
  - `Enqueue(ctx, profileID, jobID, vacancy)` — pre-checks: profile has master content, no active draft for the `(profile, job)` (409 → caller must use Rerun); `CreateDraft` with `baseline`=master snapshot + `baseline_content_hash`=sha256, enqueue `TypeGenerate` payload with `TailoringDraftID`
- [ ] T024 [US1] Extend `apps/api/internal/generation/handler.go` `Handler.ProcessTask` to detect non-nil `payload.TailoringDraftID` and dispatch to `tailoring.Service.RunProposals` instead of the merged-resume path; classify LLM errors via `llm.classifyProviderError` so FR-014 produces a clear user error (no silent auto-retry — single explicit retry offered by the dashboard)
- [ ] T025 [US1] Write HTTP handler `apps/api/internal/httpapi/tailoring.go` (`TailoringHandler` struct with `Mount(r chi.Router)` per AGENTS.md convention, implementing routes `POST /api/tailoring`, `GET /api/tailoring/{draftId}`, `POST /api/tailoring/{draftId}/proposals/{proposalId}` body `{action}`, `POST /api/tailoring/{draftId}/finalize`, `DELETE /api/tailoring/{draftId}`, `POST /api/tailoring/{draftId}/rerun` per `contracts/tailoring-api.md`); response errors map to existing `{error, path?, message?}` shape
- [ ] T026 [US1] Wire `TailoringHandler` into the server: add `composeTailoring(...)` in `apps/api/cmd/server/compose_features.go`, add `app.Tailoring` field in `apps/api/cmd/server/compose_types.go`, call it in `apps/api/cmd/server/compose.go` `buildContexts`, append `app.Tailoring.Mount` to `httpapi.NewRouter` variadic list in `apps/api/cmd/server/servers.go:buildServers`
- [ ] T027 [US1] Add latency tracking to `tailoring.Service.RunProposals` — start a timer; emit a structured log line at completion with elapsed_ms, draft_id, proposal_count; if run exceeds 30s the dashboard's existing activity polling surfaces indeterminate progress (FR-013); rejects runs > 60s with `state="error"` if `llm.ErrProviderUnavailable`/timeout (SC-007)
- [ ] T028 [P] [US1] Write dashboard `apps/dashboard/src/features/tailoring/api.ts` — typed fetch wrappers for all US1 endpoints using `@job-finder/shared` DTOs; mirror response shapes exactly
- [ ] T029 [US1] Write dashboard `apps/dashboard/src/features/tailoring/hooks.ts` — TanStack Query `useTailoringDraft`, `useAcceptProposal`, `useRejectProposal`, `useFinalizeDraft`, `useRerunDraft` mutations/queries; optimistic-update for accept/reject (rollback on error); invalidate draft query on finalize/rerun
- [ ] T030 [US1] Write `apps/dashboard/src/features/tailoring/TailoringPanel.tsx` — entry component with "Tailor resume for this job" action button (disabled when no profile/job), progress indicator after 30s (FR-013), navigable while running (non-blocking); renders `ProposalReview` once `state="review"`
- [ ] T031 [US1] Write `apps/dashboard/src/features/tailoring/ProposalReview.tsx` — grid of `EditProposal` cards grouped by `fieldType`; each card shows `beforeValue`/`afterValue` diff text and a `traceability` chip ("from master:summary" / "from job posting:required_skills"); accept/reject radio per card; "Finalize" button disabled while any `pending` remain
- [ ] T032 [US1] Mount `TailoringPanel` into the existing job-detail view `apps/dashboard/src/features/job-detail/` and (for ad-hoc) the existing ad-hoc vacancy panel — minimal integration, conditional render

**Checkpoint**: US1 fully functional end-to-end. `make test-go`, `make test-frontend`, `make test-integration` all green for US1 scenarios. Review-gated allow-list tailoring works as MVP.

---

## Phase 4: User Story 2 — Export Tailored Resume as a Single-Page PDF (Priority: P2)

**Goal**: After finalization, `POST /api/tailoring/{draftId}/export-pdf` produces a one-page text PDF via a chromedp fitter with the deterministic density ladder. If content cannot fit even at minimum density, returns `status="blocked"` with ranked actionable feedback (FR-007/008, SC-002/003/006).

**Independent Test**: Finalize a draft, `POST /export-pdf`, assert response `status="fit"` and the produced PDF has exactly 1 page (verify with `pdfcpu pages` or `pypdf.pages`) and text extraction contains profile name + accepted summary + at least one accepted bullet. Construct a deliberately over-long fixture, reject all proposals, finalize, `POST /export-pdf` → assert `status="blocked"` with non-empty `feedback[]` ranked by saved mm and NO `generated_documents` row created.

### Tests for User Story 2

- [ ] T033 [P] [US2] Write unit test `apps/api/internal/generation/singlepage/fitter_test.go` — `DensityCfg` ladder is correctly ordered largest→smallest; `block` feedback ranking algorithm prefers removing the longest bullets first then skill groups then summary, computed by measured mm removal
- [ ] T034 [P] [US2] Write integration test `apps/api/internal/generation/singlepage/integration_test.go` (`//go:build integration`) — `TestSinglePageExport_Fits` (Scenario 2 from quickstart), `TestSinglePageExport_Blocked` (Scenario 3), `TestSinglePageExport_TextPDF` (open PDF with `pdfcpu`/`pypdf`, assert 1 page + text extraction returns expected strings); serialize with `dbtest.LockSharedDB` to avoid chromedp browser-pool contention with other chromedp suites
- [ ] T035 [P] [US2] Write dashboard unit test `apps/dashboard/src/features/tailoring/ExportSinglePage.test.tsx` — success path renders download link; blocked path renders `ExportBlock[]` actionable message; pending path shows spinner and polls

### Implementation for User Story 2

- [ ] T036 [US2] Write `apps/api/internal/generation/singlepage/template.go` — embed `resume.html` (new) which renders from `dto.Resume` (NOT the legacy JSON-Resume shape): header (name, headline, location, email, phone, website, socialNetworks, customConnections), per-Section heading + entries by `entryType` (`experience`/`education`/`publication` full layout; `normal`/`text`/`bullet`/`numbered`/`reversed_numbered`/`one_line` text layout; `unrecognized` raw fields), skill groups rendered as `cv.sections.skills` (label + comma-separated details, each group one row, never strips tokens); CSS exposes `--fs`, `--m`, `--lh`, `--bg` custom properties on `:root` so the fitter can inject density values; single document flow, no columns, no flexbox that `PrintToPDF` can't replicate (per `contracts/single-page-pdf.md`)
- [ ] T037 [US2] Write `apps/api/internal/generation/singlepage/fitter.go`:
  - `Fit(ctx, resume dto.Resume, draftID uuid.UUID, outDir string, store storage.Store) (FitResult, error)`
  - Iterate the density ladder (table in `contracts/single-page-pdf.md`): for each step, execute template with the CSS vars, `page.SetDocumentContent`, `chromedp.Runtime.evaluate` `({heightPx: document.documentElement.scrollHeight, widthPx: document.documentElement.scrollWidth})`, compute `printable_height_mm = 297 − 2*marginMM`, convert measured px to mm (1px = 0.2645mm at 96dpi); fit if `measuredMM <= printable_height_mm` AND `widthPx <= printableWidthPx`
  - On first fitting step, call `page.PrintToPDF().WithPrintBackground(true)` with the matching paper/margins, write to `outDir` (`/data/documents` default), upload to MinIO via `store.Upload` if non-nil, insert a `generated_documents` row (`type="resume"`, `content` = the finalized `RendercvMaster` jsonb, `pdf_path`, `model`), return `FitResult{Status:"fit", DocumentID, FilePath, PageCount:1, MeasuredMM, DensityUsed}`
  - Exhausted ladder → return `FitResult{Status:"blocked", Feedback: rankedExportBlocks(resume, minHeightMM)}` — `rankedExportBlocks` computes per-candidate removal savings at the minimum density step (longest bullets first, then skill groups, then summary), returns the minimum set whose removal achieves fit
  - Error during chromedp → `FitResult{Status:"error"}` (caller surfaces user-facing error)
  - Synchronous budget 5s; if exceeded, return early with `Status:"pending"` and continue remaining steps in a goroutine (caller polls `/export-status`)
- [ ] T038 [US2] Extend `apps/api/internal/tailoring/service.go` with `ExportPDF(ctx, draftID)` — pre: draft `state="finalized"` and zero `pending`; call `singlepage.Fit`; on `fit` call `SetDraftExport(documentID, "fit")` and return `{documentId, status:"fit"}`; on `blocked` call `SetDraftExportStatus("blocked", feedback)` and return `{status:"blocked", feedback}`; on `pending` call `SetDraftExportStatus("pending", nil)`, kickoff background `Fit` continuation, return `{status:"pending"}`; on `error` surface clear error no silent auto-retry
- [ ] T039 [US2] Add HTTP routes to `apps/api/internal/httpapi/tailoring.go`: `POST /api/tailoring/{draftId}/export-pdf` (sync response with possibly pending), `GET /api/tailoring/{draftId}/export-status` (idempotent poll returning `{exportStatus, documentId?, feedback?}`) per `contracts/tailoring-api.md`
- [ ] T040 [US2] Extend dashboard `apps/dashboard/src/features/tailoring/api.ts` + `hooks.ts` with `useExportPdf` mutation and `useExportStatus` polling query (TanStack `refetchInterval` until terminal); wire `ExportSinglePage` state machine
- [ ] T041 [US2] Write `apps/dashboard/src/features/tailoring/ExportSinglePage.tsx` — calls `/export-pdf`, polls `/export-status`, shows spinner while `pending`, on `fit` renders a download link to the existing `/api/documents/{id}/pdf?download=1`, on `blocked` renders the `ExportBlock[]` actionable list with field labels mapped to human-readable names ("experience at Acme, bullet 3" / "Cloud skill group" / "professional summary") and a "Re-review" button back to the draft
- [ ] T042 [US2] Mount `ExportSinglePage` into `TailoringPanel` after `state="finalized"`; gate `export-pdf` button on the draft being finalized (no `pending` proposals)

**Checkpoint**: US1 + US2 functional. Resumes are not only tailorable but produce compliant one-page text PDFs (or actionable blocked messages). The product is demo-grade.

---

## Phase 5: User Story 3 — Manage Skill Groups During Tailoring (Priority: P3)

**Goal**: Refinement: explicit guardrails and UI affordances for skill-group add/remove proposals. The core generator and accept/reject already handle these (built in Phase 3); US3 closes the loop by exercising the edge-case scenarios in spec.md:70-72 (accept new group but reject removal; post-accept per-skill editing within an added group) and sur dedicated test coverage.

**Independent Test**: Trigger tailoring on a job that emphasizes a new skill domain the user has tagged but hasn't grouped → AI proposes a `skill_group_add` populated from existing skill tags (not fabricated); accept it → user can remove one skill from within the newly added group using the normal skill-edit interactions; reject a `skill_group_remove` → existing group and all its skills remain untouched.

### Tests for User Story 3

- [ ] T043 [P] [US3] Extend `apps/api/internal/generation/rendercv_proposals_test.go` with cases for US3 scenarios: (a) AI proposes `skill_group_add` with `Label="Cloud"`, `Details` containing only tokens from `masterSkillTokens`; (b) AI proposes `skill_group_remove` for an existing master label that's irrelevant to the job; (c) AI proposes `skill_change` with `AddTokens` containing tokens NOT in `masterSkillTokens` → generator suppresses that specific proposal with `dropped_reason="grounding_violation"` (no fabricated skills)
- [ ] T044 [P] [US3] Extend `apps/api/internal/tailoring/integration_test.go` with `TestTailoring_SkillGroupAddRemove` — full E2E: trigger tailoring, accept `skill_group_add`, then remove an individual skill from the added group via a per-skill dashboard mutation, verify the group persists and the targeted skill is gone; reject a `skill_group_remove` on a separate group, verify the group and all its skills remain; final review-state has correct `baselineSummary.skillGroups`

### Implementation for User Story 3

- [ ] T045 [US3] Extend `apps/api/internal/tailoring/proposals.go` per-field validators: `skill_group_add` requires `before_value=="" && after_value!=""` and the proposed `Details` token set ⊆ `master skill tokens` from the baseline (re-verify at apply-time, not just generate-time — defense in depth); `skill_group_remove` requires the label exists in `baseline` skill groups at apply-time (race-safe via `GetDraftForUpdate`)
- [ ] T046 [US3] Add `EditExistingSkillGroup` operation to `apps/api/internal/tailoring/service.go` accessed via a new HTTP route `POST /api/tailoring/{draftId}/skill-groups/{label}/skills` body `{action: "add"|"remove", token: string}` — applies a per-skill edit to an existing group in `baseline` (including groups added via a previously-accepted `skill_group_add`); enforces: tokens being added to an existing group MUST be in `masterSkillTokens` (no fabrication — FR-003/SC-005 even on user-initiated edits); persists via `UpdateDraftBaseline`; this is the "after accepting an added group, user may edit individual skills within it" interaction from clarification Q2
- [ ] T047 [US3] Add the per-skill editing route to `apps/api/internal/httpapi/tailoring.go` `Mount`; document in `contracts/tailoring-api.md` (append to existing contract file — adds one row to the route table and a request/response subsection)
- [ ] T048 [US3] Extend dashboard `apps/dashboard/src/features/tailoring/ProposalReview.tsx` to render `skill_group_add` and `skill_group_remove` proposal cards distinctly: add-card shows the proposed label + the skill chips (each chip derived from `traceability` so the user can see "from master skill tags"); once accepted, the new group appears in an "added skill groups" section with per-skill remove buttons (calls `EditExistingSkillGroup` with `action:"remove"`); remove-card shows the existing label + its current skills and a reject CTA
- [ ] T049 [US3] Add `useEditExistingSkillGroup` hook in `apps/dashboard/src/features/tailoring/hooks.ts`; optimistic update the draft's `baselineSummary.skillGroups` and the affected group's chip list

**Checkpoint**: US1 + US2 + US3 functional. Skill-group management matches the spec's US3 scenarios exactly. Atomic group accept, per-skill post-accept edits, atomic group rejection all working.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T050 [P] Run `make tygo-check` and `make sqlc-check` to confirm full typed-contract integrity across Go ↔ TS (constitution Principle III); fix any drift introduced by the new DTOs/queries
- [ ] T051 Run `make test-go` then `make test-frontend` then `make test-integration` then `make test-lint` (constitution Principle IV); fix any failures; `make test-integration` will exercise Scenarios 1-5 from `quickstart.md` against real Postgres + Redis + chromedp
- [ ] T052 [P] Update `apps/api/internal/httpapi/tailoring.go` with structured request/response logging for every route (draft_id, action, latency_ms, error_kind) — observability deferred from `/speckit.clarify` is closed here
- [ ] T053 [P] Add AI-failure integration test `apps/api/internal/tailoring/integration_test.go::TestTailoring_AIFailurePreservesBaseline` — inject a `noop` LLM provider that returns `llm.ErrProviderUnavailable`; assert draft state transitions to error, zero proposals persisted, `baseline_content_hash` unchanged (FR-014); then swap back to real Ollama and assert a single user-initiated retry succeeds (no silent auto-retry)
- [ ] T054 Run `quickstart.md` validation by hand against a running stack (via `process-hive start make run-backend` and `make run-frontend` per AGENTS.md — never inline); confirm Scenarios 1-5 behave end-to-end through the dashboard UI, not just API; capture any UX papercuts
- [ ] T055 [P] Update `apps/dashboard/src/features/tailoring/index.ts` exports and surface the TailoringPanel in the dashboard's main navigation / job-detail tab list; verify Tailwind classes match the existing dashboard theme (no new design-system deps) per `apps/dashboard/src/features/profile/` styling
- [ ] T056 Ensure the spec's version of `RendercvMaster` round-trip order (`PrepareMasterForMarshal` + `orderedYAMLMap` at `rendercv.go:564`) is NOT broken by the new skill-group add/remove merge paths — add a regression test `apps/api/internal/generation/rendercv_test.go::TestMergeTailored_SectionOrderPreserved`
- [ ] T057 [P] Document the feature in a one-paragraph block appended to `README.md` "Features" section (per repo convention — prior features have short entries); explicitly call out the constitution constraints honored (No Auto-Apply / Grounded Generation / Local-First)
- [ ] T058 Final `make test-lint` green; ready for review

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)** — no dependencies; can start immediately. T001-T004 are all `[P]` and can run in parallel.
- **Foundational (Phase 2)** — depends on Phase 1 (only the package/dir creation in T001-T003); BLOCKS all user stories. T008 (migration) ← T010 (queries) ← T009 (sqlc regen) is the serialized chain inside Phase 2; T005/T006/T007 (DTOs ↔ tygo) is a separate serialized chain; T011/T012 are `[P]` and run in parallel with the chains.
- **User Stories (Phases 3-5)** — all depend on Phase 2 completion. US1 (Phase 3) is the MVP and blocks US2's `ExportPDF` (must have a finalized draft). US3 can run in parallel with US2 but is most naturally done after US1 (its generator code is built in US1; US3 just refines tests + adds per-skill editing).
- **Polish (Phase 6)** — depends on all user stories complete.

### User Story Dependencies

- **US1 (P1)** — depends on Phase 2 only. Independent. **MVP scope**.
- **US2 (P2)** — depends on Phase 2 + a working US1 finalized-draft API surface (T038 calls into `singlepage.Fit`, which needs the finalized `RendercvMaster` baseline produced by US1's accept-loop). US2's dashboard `ExportSinglePage` mounts inside US1's `TailoringPanel`.
- **US3 (P3)** — depends on Phase 2 + US1's `skill_group_add`/`skill_group_remove` proposal code (T018, T021, T023). US3 adds per-skill post-accept editing (T046-T049) and the dedicated skill-group test coverage (T043-T044).

### Within Each User Story

- Tests first (TDD where the contract is well-defined — the grounding verifier and proposal generator are pure functions and high-value for TDD).
- LLM schema extension (T018-T020) before proposal generator (T021) before service (T023) before HTTP (T025) before dashboard (T028-T032).
- Single-page template (T036) before fitter (T037) before service `ExportPDF` (T038) before HTTP (T039) before dashboard (T040-T042).

### Parallel Opportunities

- All Setup tasks T001-T004 are `[P]`.
- Within Phase 2: T005/T006/T007 is one serialized chain; T008/T010/T009 is another; T011 and T012 are independent and `[P]` — two chains + two singletons can run in parallel.
- Within Phase 3 US1: T013-T017 (all tests) are `[P]` and can run in parallel; T018/T019/T020 are serialized (schema → prompt → verifier); T021 depends on T018-T020; T022 is `[P]`; T028 is `[P]` with T013-T017; T029-T032 serialized on the dashboard side but can run in parallel with backend tasks T023-T027.
- Within Phase 4 US2: T033-T035 tests are `[P]`; T036 → T037 → T038 → T039 → T040 → T041 → T042 is the implementation chain.
- Within Phase 5 US3: T043-T044 tests are `[P]`; T045 → T046 → T047 → T048 → T049 is the implementation chain.
- Polish: T050, T052, T053, T055, T057 are `[P]` with each other.

---

## Parallel Example: User Story 1

```bash
# Knock out all US1 test scaffolding in parallel (different files):
Task: T013 — rendercv_proposals_test.go
Task: T014 — service_test.go
Task: T015 — integration_test.go
Task: T016 — ProposalReview.test.tsx
Task: T017 — hooks.test.tsx

# Then run schema/prompt/verifier serially:
T018 → T019 → T020

# Then in parallel:
Backend: T021 → T022 → T023 → T024 → T025 → T026 → T027
Dashboard: T028 → T029 → T030 → T031 → T032
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T004) — creates empty packages/dirs, ~5 minutes.
2. Complete Phase 2: Foundational (T005-T012) — DTOs, migration, sqlc queries, queue payload extension. Critical path. `make test-go` clean at the end.
3. Complete Phase 3: User Story 1 (T013-T032) — backend + dashboard. The product now has a constrained AI tailoring flow with per-field review/accept/reject, allow-list enforced, re-run support.
4. **STOP and VALIDATE**: Run Scenario 1 + Scenario 4 + Scenario 5 from `quickstart.md`. Demo the review UI in the dashboard.
5. Ship the MVP if business-ready.

### Incremental Delivery

1. Setup + Foundational → Foundation ready, DB live, DTOs shared.
2. + US1 → MVP demoable (review-gated allow-list tailoring).
3. + US2 → Tailored resume can be exported as one-page PDF or actionable blocked feedback; demoable end-to-end.
4. + US3 → Skill-group add/remove flows match US3 spec scenarios; per-skill post-accept editing.
5. + Polish → Observability, AI-failure integration test, README feature entry, full `make test-lint` green.

### Parallel Team Strategy (if staffed)

1. Team completes Setup + Foundational together.
2. Once Foundational is done:
   - **Developer A**: US1 backend chain (T018-T027)
   - **Developer B**: US1 dashboard chain (T028-T032) — colors inside the lines of A's API contract
   - **Developer C**: US2 template + fitter (T033-T037) — can prototype the chromedp measurement against fixture data while US1 finalizes the baseline shape
3. US3 starts once US1 backend is merged (T018/T021/T023 done).
4. Polish phase parallelized across `[P]` tasks.

---

## Notes

- `[P]` tasks = different files, no dependencies on incomplete tasks.
- `[Story]` label maps task to specific user story for traceability.
- Each user story is independently testable per its Independent Test clause — Pause at any checkpoint.
- Per AGENTS.md: `pnpm --filter @job-finder/shared build` before working on the dashboard (dashboard imports the built `dist/`, not source).
- Per AGENTS.md: never commit `generated.ts` edits by hand; `tygo generate` is the source.
- Per constitution Principle I (No Auto-Apply): no task in this plan adds outbound employer-side actions; the tailoring flow only mutates the local draft + PDF artifact.
- Per constitution Principle V (Local-First): no task adds a paid external API call; the LLM router resolves to Ollama by default.
- Per AGENTS.md: never run backend/frontend via blocking Bash; if a step needs a live stack for validation, use `process-hive start make run-backend` / `make run-frontend`.