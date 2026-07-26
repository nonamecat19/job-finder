---

description: "Task list for Employer ATS Board Sources (013-ats-board-sources)"
---

# Tasks: Employer ATS Board Sources

**Input**: Design documents from `/specs/013-ats-board-sources/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/roster-api.md, quickstart.md

**Tests**: Not explicitly requested via TDD in the spec, but the project constitution (Principle
IV) requires each language's test suite to pass and cross-service behavior to be covered by
`make test-integration` against real Postgres before a change is "done." Test tasks below are
included for that reason, placed alongside the implementation they verify rather than as a
separate TDD-first block.

**Organization**: Grouped by user story (US1–US4, spec priorities P1/P1/P2/P2) so each is
independently implementable and testable.

## Path Conventions

Existing web-application monorepo layout — `apps/api/` (Go), `apps/dashboard/` (React),
`packages/shared/` (TS types). Paths below are exact per plan.md's Project Structure.

---

## Phase 1: Setup

**Purpose**: Schema and generated-code scaffolding shared by every story.

- [X] T001 Add goose migration `apps/api/internal/db/migrations/00025_ats_board_roster.sql`
  creating `EmployerBoard`, `BoardCandidate` tables, `Job.seenOnSources text[]` column, and
  `SourceRun.employerDetail jsonb` column, per data-model.md (unique constraints:
  `EmployerBoard(vendor, employerIdentifier)`, `BoardCandidate(vendor, employerIdentifier)`).
- [X] T002 Run sqlc codegen for the new tables/columns and add query definitions (roster CRUD,
  candidate CRUD, stale-counter update) in `apps/api/internal/db/queries/` per existing sqlc
  query file conventions in that directory.
- [X] T003 [P] Add `EmployerBoard`/`BoardCandidate` DTO shapes to `packages/shared` and confirm
  tygo regeneration produces matching Go types (Constitution Principle III — no hand-duplicated
  types across `apps/api`/`apps/dashboard`).

**Checkpoint**: Schema exists, generated types compile on both sides.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared code every user story's tasks build on.

**⚠️ CRITICAL**: No user story task can start until this phase is complete.

- [X] T004 Add `EmployerReporter` optional interface (`LastRunDetail() []EmployerRunOutcome`) to
  `apps/api/internal/jobsources/adapter.go`, next to the existing `DetailNeeder` interface, per
  research.md §2.
- [X] T005 Create `roster.Service` in `apps/api/internal/jobsources/roster/service.go`:
  `List`, `Get(vendor, employerIdentifier)`, `Create`, `Delete`, `RecordRunOutcome` (updates
  `lastSuccessAt`/`lastPostingCount`/`consecutiveEmptyRuns`), and a `stale` computation
  (`consecutiveEmptyRuns >= threshold`, threshold as a package constant) per data-model.md.
- [X] T006 [P] Create vendor URL matcher in
  `apps/api/internal/jobsources/roster/urlmatch.go`: `MatchVendor(rawURL string) (vendor,
  employerIdentifier string, ok bool)` covering the 5 host patterns from research.md §1
  (`boards.greenhouse.io`, `jobs.lever.co`, `jobs.ashbyhq.com`, `apply.workable.com`,
  `jobs.smartrecruiters.com`), with a unit test in
  `apps/api/internal/jobsources/roster/urlmatch_test.go`.
- [X] T007 [P] Create per-vendor read client scaffolding in
  `apps/api/internal/jobsources/adapters/atsboard.go`: shared HTTP helpers for the 5 board
  endpoints (reusing `jobsources.GetJSON` per the existing adapter pattern) so each vendor file
  only defines its response struct and normalization, not its own HTTP plumbing.
- [X] T008 Register `roster.Service` construction in `apps/api/cmd/server/compose.go` and
  `apps/api/cmd/seed/main.go`, mirroring existing adapter-registration call sites.

**Checkpoint**: Interfaces, roster service, URL matcher, and shared HTTP scaffolding exist —
user story work can begin.

---

## Phase 3: User Story 1 - Discover listings straight from employer job boards (Priority: P1) 🎯 MVP

**Goal**: Each of the 5 board vendors reads every roster employer's open postings, unauthenticated,
and ingests them with title/company/location/remote/apply URL/description populated, scored on
first ingest.

**Independent Test**: Register one employer per vendor directly (bypassing candidate discovery —
use the roster insert path), trigger each vendor's source run, confirm new `Job` rows appear
attributed to that vendor with all required fields populated.

### Implementation for User Story 1

- [X] T009 [P] [US1] Implement `GreenhouseAdapter` in
  `apps/api/internal/jobsources/adapters/greenhouse.go`: `Search` fans out over
  `roster.Service.List(vendor="greenhouse")`, calls `boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true`
  per employer (capped per FR-007), normalizes into `dto.NormalizedJob`, implements
  `EmployerReporter`. Unit test `greenhouse_test.go` covering a fixture response, a 404 (board
  not found), and a malformed-shape response.
- [X] T010 [P] [US1] Implement `LeverAdapter` in
  `apps/api/internal/jobsources/adapters/lever.go` against
  `api.lever.co/v0/postings/{token}?mode=json`, same shape as T009, with `lever_test.go`.
- [X] T011 [P] [US1] Implement `AshbyAdapter` in
  `apps/api/internal/jobsources/adapters/ashby.go` against
  `api.ashbyhq.com/posting-api/job-board/{token}`, same shape as T009, with `ashby_test.go`.
- [X] T012 [P] [US1] Implement `WorkableAdapter` in
  `apps/api/internal/jobsources/adapters/workable.go` against
  `apply.workable.com/api/v1/widget/accounts/{token}`, same shape as T009, with
  `workable_test.go`.
- [X] T013 [P] [US1] Implement `SmartRecruitersAdapter` in
  `apps/api/internal/jobsources/adapters/smartrecruiters.go` against
  `api.smartrecruiters.com/v1/companies/{token}/postings`, same shape as T009, with
  `smartrecruiters_test.go`.
- [X] T014 [US1] Register all 5 adapters as `JobSource` entries in `apps/api/cmd/server/compose.go`
  and `apps/api/cmd/seed/main.go` (depends on T009–T013).
- [X] T015 [US1] Extend `ingestion.Handler.ProcessTask` in `apps/api/internal/ingestion/handler.go`
  to type-assert `EmployerReporter` after `adapter.Search`, persist per-employer outcomes into
  `SourceRun.employerDetail`, call `roster.Service.RecordRunOutcome` per employer, and set
  `SourceRun.ok = employersReadSuccessfully > 0` (FR-019, FR-020, FR-021). Depends on T004, T005,
  T014.
- [X] T016 [US1] Verify existing per-host pacing (`apps/api/internal/jobsources/util.go` or
  equivalent rate limiter already used by other adapters) is applied to the 5 new board hosts —
  reuse, do not introduce a second pacing mechanism (FR-006).
- [ ] T017 [US1] Integration test in `apps/api/internal/ingestion/` (or
  `apps/api/test/integration/`, matching existing integration test location) that runs each of
  the 5 vendor sources against fixture/mock HTTP servers end-to-end and asserts `Job` rows with
  all required fields, no duplicate on re-run, and no error when a previously-seen posting
  disappears (spec Acceptance Scenarios 1–3, 5).

**Checkpoint**: User Story 1 fully functional — run any of the 5 vendor sources against a
directly-registered employer and see new jobs in the feed.

---

## Phase 4: User Story 2 - Build the employer roster without manual research (Priority: P1)

**Goal**: System proposes roster candidates from existing listings' apply URLs; user accepts,
rejects, or pastes a board URL directly.

**Independent Test**: With existing listings linking to employer boards, run candidate discovery,
confirm proposed candidates name the right employer/vendor, and confirm accepting one causes that
board's postings to appear on the next run.

### Implementation for User Story 2

- [X] T018 [US2] Implement candidate discovery in
  `apps/api/internal/jobsources/roster/candidates.go`: `Discover(ctx)` scans `Job.url` via
  `roster.MatchVendor` (T006), inserts new `BoardCandidate(state=proposed)` rows, skips URLs
  already matching an `EmployerBoard` or existing `BoardCandidate` (FR-013, Edge Case: no
  re-offer). Depends on T006.
- [X] T019 [US2] Implement `Accept(id)` / `Reject(id)` on the candidate service: accept creates/
  enables the matching `EmployerBoard` row and marks the candidate `accepted`; reject marks
  `rejected`, terminal (FR-010).
- [X] T020 [US2] Implement `RegisterFromURL(ctx, url)` on `roster.Service`: resolves vendor via
  `MatchVendor`, rejects unsupported vendors with the 5-vendor list in the error, performs a live
  single-employer health-check read before insert (FR-011), returns the created `EmployerBoard` or
  a structured `unreadable`/`unsupported_vendor` error per contracts/roster-api.md.
- [X] T021 [US2] Create `apps/api/internal/httpapi/roster.go`: `RosterHandler.Mount` wiring
  `GET /api/roster`, `POST /api/roster`, `DELETE /api/roster/{id}`, `GET /api/roster/candidates`,
  `POST /api/roster/candidates/{id}/accept`, `POST /api/roster/candidates/{id}/reject`,
  `POST /api/roster/discover` per contracts/roster-api.md request/response shapes exactly.
  Depends on T018–T020.
- [X] T022 [US2] Mount `RosterHandler` in the server's router setup (same file/pattern as
  `SourcesHandler.Mount` is wired today).
- [ ] T023 [P] [US2] Add `useRoster`, `useCandidates`, `useAcceptCandidate`, `useRejectCandidate`,
  `useRegisterBoard`, `useDiscoverCandidates` hooks in
  `apps/dashboard/src/features/sources/roster/hooks.ts`.
- [ ] T024 [US2] Build `RosterPanel` and `CandidatesPanel` components in
  `apps/dashboard/src/features/sources/roster/` (paste-a-URL form with inline error display,
  candidate accept/reject buttons) and mount them on
  `apps/dashboard/src/features/sources/SourcesPage.tsx`. Depends on T023.
- [ ] T025 [US2] Integration test covering: seed `Job` rows with board-vendor apply URLs → run
  discovery → assert correct candidates → accept one → run its source → assert postings appear;
  and reject one → re-run discovery → assert it does not reappear (spec Acceptance Scenarios
  1–3). Also cover pasting an unsupported-vendor URL (Edge Case) and pasting an unreadable board
  URL (Acceptance Scenario 5).

**Checkpoint**: Roster can be built entirely from proposed candidates or pasted URLs; User
Stories 1 and 2 together deliver a working, self-populating source family.

---

## Phase 5: User Story 3 - Recognise the same job arriving from two places (Priority: P2)

**Goal**: A posting seen via an aggregator and later via an employer board merges into one `Job`
row, preferring the board's apply URL, preserving prior user state, without over-merging distinct
openings.

**Independent Test**: Ingest a posting from an aggregator, then the same posting from the
employer's board, confirm one job with the board's apply URL and both origins recorded.

### Implementation for User Story 3

- [ ] T026 [US3] Extend `apps/api/internal/ingestion/dedupe.go` with a merge-candidate check per
  research.md §4: given a new `NormalizedJob` whose `DedupeKey` does not already exist, find an
  existing `Job` from a different `sourceKey` with matching normalized `company` + high embedding
  similarity to the new posting's description, returning a merge candidate `Job.id` or none.
- [ ] T027 [US3] Extend `persistIfNew` in `apps/api/internal/ingestion/handler.go`: when a merge
  candidate is found and the new source is an employer board, UPDATE the existing `Job` row —
  set `url`/`sourceKey` to the board's, append the new source to `seenOnSources` — instead of
  inserting a new row; when the new source is not a board (or no candidate found), keep existing
  insert-or-repost behavior unchanged. Depends on T026.
- [ ] T028 [US3] Integration test: ingest an aggregator posting, save/score/move it (create
  `Application`/`MatchResult` rows), then ingest the matching employer-board posting, assert one
  `Job` row remains with the board's `url`, `seenOnSources` containing both source keys, and the
  prior `Application`/`MatchResult` rows still pointing at the same `Job.id` untouched (FR-016,
  FR-017, spec Acceptance Scenarios 1–3).
- [ ] T029 [US3] Integration test: ingest two postings with identical titles at different
  companies, and two genuinely separate reqs for the same role at one employer, assert both stay
  as distinct `Job` rows (FR-018, spec Acceptance Scenario 4).

**Checkpoint**: Aggregator and board copies of the same opening collapse into one job; distinct
openings never incorrectly merge.

---

## Phase 6: User Story 4 - Manage board sources like every other source (Priority: P2)

**Goal**: Each vendor is a normal `JobSource` on the Sources screen (enable/disable/run/health-
check); a failing employer never blocks others; the roster view surfaces staleness.

**Independent Test**: From the Sources screen, enable a board vendor, run it on demand, watch its
health check pass, read its resulting counts, disable it, confirm the next scheduled run skips it.

### Implementation for User Story 4

- [X] T030 [US4] Implement `HealthCheck` on each of the 5 adapters (T009–T013 follow-up if not
  already done there): a lightweight read (e.g. first roster employer, or a fixed known-good
  probe) satisfying the existing `Adapter.HealthCheck` contract used by `POST /sources/{key}/test`.
- [ ] T031 [US4] Confirm the 5 new `JobSource` rows appear via the existing
  `GET /sources` / `PUT /sources/{key}` / `POST /sources/{key}/test` / `POST /sources/{key}/run`
  endpoints with no new endpoint code required (FR-022) — add a dashboard smoke check only if
  `SourcesPage.tsx` needs a vendor-icon/label addition for the 5 new keys.
  `apps/dashboard/src/features/sources/SourcesPage.tsx`.
- [ ] T032 [US4] Add `lastSuccessAt`, `lastPostingCount`, and `stale` columns to the `RosterPanel`
  (T024) table view (FR-024).
- [ ] T033 [US4] Integration test: run a vendor source where one employer in the roster returns a
  bad/expired token and others succeed; assert the run's `employerDetail` marks that employer
  `unreadable`/`refused`/`not_found` distinctly, the run's aggregate `found`/`new` still reflect
  the successful employers, and `ok` stays true (FR-019, FR-020, spec US4 Acceptance Scenario 3).
- [ ] T034 [US4] Integration test: an employer with `consecutiveEmptyRuns` at or above the stale
  threshold shows `stale=true` from `GET /api/roster`; one successful run resets the counter to 0
  (FR-014, spec Edge Cases).
- [ ] T035 [US4] Integration test: a run where every employer fails (e.g. all boards unreachable)
  asserts `SourceRun.ok = false` (FR-021).

**Checkpoint**: All 4 user stories independently functional; operators can run, debug, and
maintain the roster entirely from the Sources screen.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T036 Run `specs/013-ats-board-sources/quickstart.md` end-to-end against a local
  `make up` stack and confirm every step's expected result.
- [ ] T037 [P] `make test-lint` (both `apps/api` `go test` and `apps/dashboard` `vitest` suites)
  passes with all changes from this feature included (Constitution Principle IV).
- [ ] T038 [P] `make test-integration` passes, covering T017, T025, T028, T029, T033–T035.
- [ ] T039 Re-check Constitution gates in `plan.md`'s Constitution Check section against the
  final diff (Principle III: no hand-duplicated roster/candidate types slipped into
  `apps/dashboard` outside `packages/shared`).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup (needs the migrated schema for `roster.Service`
  and sqlc queries) — BLOCKS all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational only. No dependency on US2/US3/US4.
- **User Story 2 (Phase 4)**: Depends on Foundational only. Independently testable via direct
  roster inserts even before US1's adapters exist, though exercising "postings appear on next
  run" (Acceptance Scenario 2) needs at least one adapter from US1 — sequence US1 before US2 in
  practice even though they're both P1 and structurally independent.
- **User Story 3 (Phase 5)**: Depends on Foundational; exercising its integration tests needs a
  board adapter (US1) and an existing aggregator adapter (already in the codebase) to have both
  ingested — sequence after US1.
- **User Story 4 (Phase 6)**: Depends on Foundational and the per-employer reporting introduced
  in US1 (T015); roster staleness display depends on US2's `RosterPanel` (T024).
- **Polish (Phase 7)**: Depends on all desired user stories being complete.

### Within Each User Story

- Adapters/services before endpoints/UI before integration tests.
- T009–T013 (5 adapters) are parallel; T014 depends on all five.

### Parallel Opportunities

- T009–T013 (5 vendor adapters, distinct files) — fully parallel.
- T006 and T007 (Foundational) — parallel, distinct files.
- T023 (dashboard hooks) can start once T021's endpoint shapes are fixed by contracts/roster-api.md,
  without waiting for T021's implementation to land.

---

## Parallel Example: User Story 1

```bash
Task: "Implement GreenhouseAdapter in apps/api/internal/jobsources/adapters/greenhouse.go"
Task: "Implement LeverAdapter in apps/api/internal/jobsources/adapters/lever.go"
Task: "Implement AshbyAdapter in apps/api/internal/jobsources/adapters/ashby.go"
Task: "Implement WorkableAdapter in apps/api/internal/jobsources/adapters/workable.go"
Task: "Implement SmartRecruitersAdapter in apps/api/internal/jobsources/adapters/smartrecruiters.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup
2. Phase 2: Foundational
3. Phase 3: User Story 1 — register employers directly (no candidate UI yet), run each vendor,
   confirm jobs ingest.
4. **STOP and VALIDATE** against quickstart.md §1.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → 5 working readers, manually-registered roster → demo (MVP).
3. US2 → self-populating roster via candidates/paste → demo.
4. US3 → merge behavior → demo, feed stops duplicating.
5. US4 → Sources-screen parity, stale flagging, failure isolation → operational maturity.
6. Polish → quickstart + full test suites green.

## Notes

- [P] tasks touch different files with no unmet dependency.
- Every board-vendor adapter returns full descriptions inline (FR-004) — none should implement
  `DetailNeeder`; this is a thing to verify in T009–T013's tests, not a separate task.
- Commit after each task or logical group, per repo convention (small per-feature commits).
