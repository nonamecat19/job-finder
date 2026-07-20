---
description: "Task list for recruiter / hiring-manager resolution"
---

# Tasks: Recruiter / Hiring-Manager Resolution

**Input**: Design documents from `/specs/007-recruiter-resolution/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Included. Constitution Principle IV makes them a gate — a change is not "done" until its language's suite passes locally, and `make test-lint` gates a merge crossing `apps/api` + `apps/dashboard`. The no-fabrication tests (Principle II) are non-negotiable, not optional coverage.

**Organization**: Grouped by user story so each ships independently.

## ⚠️ DEPENDENCY GATE — plan 004 must land first

**No implementation task below may start until plan 004's `internal/companyintel` package (task 004-4) has landed.** The company-page source (US2) reuses its fetch. The spec bundle (this task, 007-1) has no such gate and is already written. Some tasks that touch no company-page code (the migration, the posting-text source) *could* technically start earlier, but the plan keeps a single clean gate to avoid a half-landed feature depending on an unmerged package.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths included per task

## Path Conventions

All paths are repo-relative from the repo root. Backend in `apps/api` (Go), UI in `apps/dashboard` (React). One new migration; no `packages/shared` hand-editing (tygo regenerates any exposed type).

---

## Phase 1: Setup (Shared Infrastructure)

- [ ] T001 Confirm plan 004's `apps/api/internal/companyintel` package has landed and exposes a reusable company-page fetch; if not, STOP — the gate above is unmet.
- [ ] T002 [P] Add `LinkedInScrapeEnabled bool \`env:"LINKEDIN_SCRAPE_ENABLED" envDefault:"false"\`` to `apps/api/internal/config/config.go` — comment that LinkedIn scraping is a ToS gray area, default off, decision placed with the operator (plan.md Constitution Check).
- [ ] T003 [P] Create `apps/api/internal/recruiter/testdata/` and capture the company-page fixture (About/Team HTML); record capture date in a comment. Posting-text fixtures are inline test strings, not files.

**Checkpoint**: config field present; fixtures available for unit tests.

---

## Phase 2: Foundational (Blocking Prerequisites — the JobContact table)

**⚠️ CRITICAL**: No user story work can begin until the table exists.

> **Note**: the migration is also tracked as its own workspace task ("Add the JobContact table", el-1ohn). T004-T006 restate it here for ordering completeness; do not double-implement — land it once.

- [ ] T004 Create `apps/api/internal/db/migrations/00010_job_contact.sql` (next free sequential goose version if 00010 is taken) with the `JobContact` table exactly per [data-model.md](./data-model.md): PK `id`, FK `jobId` → `Job(id)` `ON DELETE CASCADE`, columns `name NOT NULL`, `title`, `linkedInUrl`, `email`, `phone`, `source NOT NULL`, `confidence NOT NULL`, `fetchedAt`; `UNIQUE(jobId, source, name)`; index `JobContact_jobId_idx`. Goose Up/Down markers.
- [ ] T005 Update `apps/api/internal/db/integration_test.go:truncateAll` to truncate `JobContact` **before** `Job` (FK cascade ordering), matching the existing child-table pattern.
- [ ] T006 Add sqlc queries in `apps/api/internal/db/queries/job_contact.sql`: `UpsertJobContact` (insert-or-update on `(jobId, source, name)` per FR-013), `ListJobContactsByJob` (ordered confidence desc, then source priority, then name — the deterministic order for FR-010/SC-010), and regenerate sqlc.

**Checkpoint**: `goose up`/`down` clean against `TEST_DATABASE_URL`; `go test ./internal/db/...` passes; upsert + cascade covered by integration tests (SC-006, SC-009). Commit here.

---

## Phase 3: User Story 1 - See the resolved contact on the job detail page (Priority: P1) 🎯 MVP

**Goal**: The posting-text source resolves a contact and the detail page shows it (or the "No contact found" state).

**Independent Test**: Ingest a job whose body names a contact, open the detail page, see "Jane Doe — Recruiter". Ingest one that names no one, see "No contact found — try Refresh".

### Tests for User Story 1

- [ ] T007 [P] [US1] `TestPostingParseNamedContact` in `apps/api/internal/recruiter/posting_test.go` — a body with `Contact: Jane Doe, Recruiter <jane@acme.com>` yields name/title/email, `Source=="posting"` (SC-001).
- [ ] T008 [P] [US1] `TestPostingNoContact` and `TestPostingGenericMailbox` — no-contact body yields zero rows, no error (SC-002); `jobs@acme.com`-only body yields **no named contact** (FR-007, SC-003).
- [ ] T009 [P] [US1] `TestPostingFieldTraceability` — a body with a phone but no name yields no name-bearing row and no field absent from input (FR-008, Principle II). **Non-negotiable.**
- [ ] T010 [P] [US1] `TestPostingCyrillic` — a Cyrillic contact line round-trips byte-identical.

### Implementation for User Story 1

- [ ] T011 [US1] Define `ResolvedContact` (fields per [data-model.md](./data-model.md)) in `apps/api/internal/recruiter/resolve.go`.
- [ ] T012 [US1] Implement the posting-text source in `apps/api/internal/recruiter/posting.go` — local-LLM extraction over `Job.description`, grounded (emit only observed spans), regex-validate email/phone, drop generic mailboxes as named contacts, assign `Source="posting"` and confidence.
- [ ] T013 [US1] Implement `Resolve(ctx, jobID)` orchestration in `apps/api/internal/recruiter/resolve.go` — run the posting source, upsert via `UpsertJobContact`, return without error on zero contacts (FR-016). (Company-page/LinkedIn added in US2.)
- [ ] T014 [US1] Add `GET /jobs/{id}/contacts` in `apps/api/internal/httpapi/` returning contacts ordered best-first; channels never logged in full (FR-018).
- [ ] T015 [US1] Add the **Contact** line to the job detail page in `apps/dashboard/src/` — show the headline contact (highest confidence) or "No contact found — try Refresh" (FR-009); vitest coverage for both states.

**Checkpoint**: quickstart Levels 1 & 3 (steps 1-2) pass. MVP demoable. Commit here.

---

## Phase 4: User Story 2 - Refresh contacts on demand (Priority: P2)

**Goal**: A Refresh action re-runs all enabled sources, including the company-page source and (opt-in) LinkedIn.

### Tests for User Story 2

- [ ] T016 [P] [US2] `TestCompanyPageParse` in `apps/api/internal/recruiter/companypage_test.go` over the saved fixture — ≥1 contact, `Source=="company-page"`; a no-People-section fixture yields zero, no error (edge case).
- [ ] T017 [P] [US2] `TestLinkedInSkippedWhenDisabled` — with `LinkedInScrapeEnabled=false`, the LinkedIn source is never invoked and makes zero requests (FR-004, SC-004).
- [ ] T018 [P] [US2] `TestResolveOneSourceFails` — a failing source leaves the others' contacts persisted; run not marked failed (FR-015, SC-007).
- [ ] T019 [P] [US2] `TestResolveIdempotent` (integration) — two runs on unchanged data leave row count unchanged (FR-013, SC-006).

### Implementation for User Story 2

- [ ] T020 [US2] Implement the company-page source in `apps/api/internal/recruiter/companypage.go` — reuse plan 004's `internal/companyintel` fetch on `Company.website`; skip (not fail) when website absent; LLM-parse Team/About for contacts; `Source="company-page"`, lower confidence than a posting `Contact:` line.
- [ ] T021 [US2] Implement the LinkedIn source in `apps/api/internal/recruiter/linkedin.go` — **gated on `LinkedInScrapeEnabled`**; when off, never constructed/invoked. Public company-page People section only, read-only, shared-pacing fetch; degrade to zero + warning on block/markup change; `Source="linkedin"`.
- [ ] T022 [US2] Extend `Resolve` in `resolve.go` to run all three sources with per-source error isolation (FR-015), then upsert all results.
- [ ] T023 [US2] Add `POST /jobs/{id}/contacts/refresh` in `apps/api/internal/httpapi/` — re-runs `Resolve`, returns the updated set.
- [ ] T024 [US2] Add the **Refresh contacts** button to the detail page in `apps/dashboard/src/` — invalidates the contacts query and updates the line in place, no reload (SC-005); vitest coverage.

**Checkpoint**: quickstart Level 3 (steps 3, 5, 6) + Level 4 failure table pass. Commit here.

---

## Phase 5: User Story 3 - Expand the full candidate list (Priority: P3)

**Goal**: The Contact line expands to every resolved contact, each with source and confidence.

### Tests for User Story 3

- [ ] T025 [P] [US3] `TestListOrderingDeterministic` — contacts from multiple sources sort by confidence desc with the stable tie-break; identical order across repeated reads (FR-010, SC-010).

### Implementation for User Story 3

- [ ] T026 [US3] Add the expandable list to the detail page in `apps/dashboard/src/` — every stored contact with name, title, channels, **source label**, and **confidence indicator**, ordered best-first (FR-012, SC-008); vitest coverage including the single-contact case (Story 3 scenario 3).

**Checkpoint**: quickstart Level 3 (step 4) passes. Commit here.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T027 [P] Add `TestLive_CompanyPage` and (opt-in) `TestLive_LinkedIn` to a `//go:build live` file in `apps/api/internal/recruiter/` — the only tests that catch markup drift. `TestLive_LinkedIn` skips unless `LINKEDIN_SCRAPE_ENABLED=true`.
- [ ] T028 [P] Add a package doc comment to `apps/api/internal/recruiter/` in house style — three sources, posting always-on, LinkedIn opt-in/default-off ToS note, read-only (no outreach).
- [ ] T029 Verify Constitution Principle I — grep `internal/recruiter/` for any non-GET request or any message/email/apply-to-contact call; there must be none (FR-017).
- [ ] T030 Verify Constitution Principle II — confirm the no-fabrication tests (T008/T009) exist and pass; confirm no channel is logged in full (FR-018).
- [ ] T031 Run `cd apps/api && go test ./...`, the dashboard vitest suite, and `make test-lint` — the boundary gate (change spans two apps).
- [ ] T032 Walk quickstart Levels 3-4 end-to-end against the running stack, including the failure-mode table.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: T001 is the plan-004 gate check — blocks everything.
- **Foundational (Phase 2)**: the `JobContact` table (also tracked as el-1ohn). **BLOCKS all user stories.**
- **US1 (Phase 3)**: depends on Foundational. Posting-text source only — the MVP.
- **US2 (Phase 4)**: depends on Foundational **and plan 004** (company-page fetch). Reuses `Resolve` from US1.
- **US3 (Phase 5)**: depends on Foundational; richer once US2 provides multi-source data, but the list/ordering works with US1 data alone.
- **Polish (Phase 6)**: depends on the desired stories.

### Parallel Opportunities

- T002/T003 parallel (different files).
- All US1 tests (T007-T010) parallel — inline fixtures, all fail until T011-T013 land.
- T016-T019 (US2 tests) parallel with each other.
- T027/T028 parallel.
- **Caveat**: tasks touching the same file (`resolve.go`, `posting.go`, the detail-page component) are sequential even within a phase.

### Story Independence

US1 alone is a shippable MVP: postings that name a contact get a Contact line, powered entirely by data already in the DB — no external fetch, no opt-in. US2 and US3 add coverage and depth without breaking it.

---

## Notes

- `[P]` = different files, no dependencies. Same-file tasks are never `[P]`.
- Zero contacts is success, never an error (FR-016).
- Never fabricate a person; a generic mailbox is not a human (FR-007). The Principle-II tests are the gate that enforces it.
- LinkedIn stays off unless the operator sets `LINKEDIN_SCRAPE_ENABLED=true` (FR-004). Silent skip when off — never a failed run.
- No `packages/shared` hand-editing; any exposed contact type regenerates via tygo (Principle III).
- Do not fork plan 004's company-page fetch — import it (`internal/companyintel`).
