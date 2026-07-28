# Quickstart — Feature 020 validation guide

This guide documents the runnable end-to-end scenarios that prove the feature works. It is a *validation run guide*, not a task list — implementation steps live in `tasks.md` (produced by `/speckit.tasks`).

## Prerequisites

- Working `job-finder` dev environment (Docker Compose stack up: `make up`).
- Ollama running locally with the model configured for the `generation` task (`make run-backend` will wire it; verify `GET /v1/settings/llm/models`).
- A user **profile** with a working master resume. If none, create one via the 009 Profile UI (or seed: `make seed`).
- A saved **job posting** to tailor against (use existing `jobs` test fixtures or save one from a search). Ad-hoc variant (paste vacancy) requires only the profile.
- Build ordered dependencies: `pnpm install && pnpm --filter @job-finder/shared build` (per `AGENTS.md`), then `make run-backend` / `make run-frontend` (via `process-hive` start, not inline shell).

## Scenario 1 — Tailor resume end-to-end, allow-list enforced

**Goal**: prove the core US1 path — AI proposes edits only to allow-listed fields, user accepts/rejects, and the diff matches the spec.

**Steps**:
1. `POST /api/tailoring` with `{profileId, jobId}` → expect **202** `{draftId, activityId}`.
2. Poll `GET /api/tailoring/{draftId}` every ~3s until `state="proposing"` then `"review"` (≤ 60s typical — SC-007). The `activity` row appears in the existing activity feed (FR-013). After 30s assert the dashboard shows indeterminate progress.
3. Once `state="review"`: every `proposals[].fieldType` MUST be one of `summary | experience_highlights | skill_change | skill_group_add | skill_group_remove`. **No other field type is permitted (SC-001)**.
4. Cross-check `proposals[].afterValue` against the master resume's `cv.sections` keys other than allow-listed ones: assert **no** diff in job titles, employer names, dates, education entries, certs, links. This is enforced by the grounding verifier (research R8) — a violation here is a critical bug.
5. Pick one `experience_highlights` proposal and `POST .../proposals/{id}` `{action:"reject"}`. Assert the next `GET /draft` shows the proposal `status="rejected"` and `baselineSummary` for that bullet is unchanged (FR-006).
6. Pick the `summary` proposal and `POST .../proposals/{id}` `{action:"accept"}`. Assert `proposal.status="accepted"` AND `tailored_drafts.baseline` summary text now equals `afterValue` (query directly or via the `GET /draft` `baselineSummary`).

**Expected outcome**: A draft in `state="review"` with pending proposals, allow-list 100% enforced (SC-001), accept/reject mutate the baseline correctly.

## Scenario 2 — Single-page PDF export, fits

**Goal**: prove US2 happy path — finalized resume exports as a one-page text PDF.

**Steps**:
1. From Scenario 1: `POST /api/tailoring/{draftId}/finalize`. Assert `state="finalized"` and zero `pending` proposals remain.
2. `POST /api/tailoring/{draftId}/export-pdf`. Assert synchronous **200** with `status="fit"`, `documentId` non-null, `exportStatus="fit"` (SC-002).
3. `GET /api/documents/{documentId}/pdf`. Assert response `Content-Type: application/pdf`, body is a PDF whose page count is **1** (verify with `pdfcpu pages <file>` or `python -c "from pypdf import PdfReader; print(len(PdfReader('<file>').pages))"`).
4. Extract text from the PDF (`pdftotext <file> -`) and assert the profile name, accepted summary, and at least one accepted bullet are present as selectable text (FR-007 / ATS guarantee).

**Expected outcome**: a 1-page text PDF persisted via `generated_documents`, downloadable via the existing `/api/documents/{id}/pdf` route.

## Scenario 3 — Single-page export blocked, actionable feedback

**Goal**: prove the impossible-case branch (FR-008 / SC-006).

**Steps**:
1. Construct a fixture profile with deliberately over-long resume: ≥ 12 experience bullets per company, ≥ 8 skill groups, summary ≥ 800 characters. (For a one-off test fixture, insert directly via the profile PUT endpoint.)
2. Trigger tailoring as Scenario 1.
3. **Reject** all proposals (so baseline equals master), then `POST /finalize` followed by `POST /export-pdf`.
4. Assert response `status="blocked"` and `exportStatus="blocked"`, with `feedback[]` non-empty. Each `ExportBlockDto` MUST have `field` enumerated by `experience:<company>:<i>` / `skill_group:<label>` / `summary` (matches single-page-pdf.md contract).
5. Assert NO `generated_documents` row was created (`state="finalized"`, `export_document_id` null).

**Expected outcome**: no PDF produced, clear actionable feedback, **no** truncated multi-page PDF silently emitted (SC-006).

## Scenario 4 — Re-run does not re-surface accepted edits

**Goal**: prove clarification Q4 / FR-010.

**Steps**:
1. From Scenario 1's accepted-summary draft (state `finalized`): `POST /api/tailoring/{draftId}/rerun`.
2. Assert **202** returns a new `draftId`; the old draft remains `finalized`.
3. Poll elements of `GET /tailoring/{newDraftId}/proposals` once `state="review"`. **Assert:** the summary proposal from the first run is NOT present in any `pending` list (it's part of the seeded baseline now). New proposals must reference different fields or a different rewording.

**Expected outcome**: re-run uses the current baseline as the diff target, no ghost re-proposals of accepted edits.

## Scenario 5 — AI failure handling preserves baseline

**Goal**: prove FR-014 graceful error handling.

**Steps**:
1. Stop the Ollama container (`docker compose stop ollama`) so the `generation` worker fails to reach the model.
2. `POST /api/tailoring` with a valid profile/job.
3. Poll the draft. Assert the run ends in a terminal activity state (`failed` or `timed_out` per feature 019's activity states), the draft `state` transitions back to `drafting` or a new `error` surface shown to the user — **no partial proposals are persisted**, **no** `baseline` mutation occurred (compare `baseline_content_hash`). The dashboard surfaces a clear error + a single "Retry" action (no silent auto-retry — FR-014).
4. Restart Ollama, click "Retry". The subsequent run produces proposals normally (Scenario 1).

**Expected outcome**: baseline preserved on AI failure, single explicit retry offered, no silent auto-retrials.

## Integration tests

These scenarios are codified (not run by hand) under `apps/api/internal/tailoring/integration_test.go` (`//go:build integration`):
- `TestTailoring_AllowListEnforced` — Scenario 1.
- `TestTailoring_SinglePageExport_Fits` — Scenario 2 (uses real chromedp).
- `TestTailoring_SinglePageExport_Blocked` — Scenario 3.
- `TestTailoring_RerunDoesNotResurfaceAccepted` — Scenario 4.
- `TestTailoring_AIFailurePreservesBaseline` — Scenario 5 (uses the existing `dbtest` harness; Ollama failure simulated via a `noop` LLM provider injected in the test composition).

Run them via `make test-integration` (real Postgres + Redis + chromedp via Docker Compose, per constitution Principle IV). The single-page-Chrome integration test serializes with other chromedp tests using `dbtest.LockSharedDB` to avoid browser-pool contention.

## Unit tests (per language, fast)

- `apps/api/internal/tailoring/service_test.go` — proposal generator (partial payloads, drop/accept, baseline propagation, re-run seeding).
- `apps/api/internal/generation/singlepage/fitter_test.go` — density-ladder ordering, blocked-feedback ranking.
- `apps/api/internal/generation/rendercv_proposals_test.go` — LLM payload → `[]EditProposal` translation, grounding-suppression (`dropped`).
- `apps/dashboard/src/features/tailoring/*.test.tsx` — ProposalReview card accept/reject, ExportSinglePage blocked-message rendering, TanStack Query hook contract.

## Final gates

- `make test-go` — all new Go unit tests pass.
- `make test-frontend` — vitest passes.
- `make test-integration` — Scenarios 1-5 green against real Postgres + Redis + chromedp.
- `make tygo-check` and `make sqlc-check` — typed-contracts integrity (constitution Principle III).
- `make test-lint` before merge.