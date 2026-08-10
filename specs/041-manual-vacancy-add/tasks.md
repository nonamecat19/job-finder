---
description: "Task list for 041-manual-vacancy-add"
---

# Tasks: Manual Vacancy Add by URL

**Input**: Design documents from `/specs/041-manual-vacancy-add/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/manual-add.md, quickstart.md

**Tests**: Included. Constitution Principle IV binds this repo to per-language suites, and
cross-service behaviour must be exercised against real Postgres/Redis rather than mocks. Test
tasks are therefore not optional here.

**Organization**: Grouped by user story. US1 alone is a shippable MVP.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on incomplete work)
- **[Story]**: US1 / US2 / US3, mapping to the spec's user stories
- Paths are repo-relative and exact

## Path Conventions

Go API at `apps/api/`, React dashboard at `apps/dashboard/`, shared TS types at
`packages/shared/`. Generated files (`apps/api/internal/db/sqlcgen/`,
`packages/shared/src/generated.ts`) are regenerated, never hand-edited.

---

## Phase 1: Setup

**Purpose**: Skeleton and fixtures the rest of the work lands in

- [X] T001 Create the manualadd module skeleton — `apps/api/internal/manualadd/domain/`, `apps/api/internal/manualadd/application/`, `apps/api/internal/manualadd/interfaces/http/` — plus `apps/api/internal/manualadd/manualadd.go` re-exporting the public surface, following the pattern of `apps/api/internal/subscriptions/subscriptions.go`
- [X] T002 [P] Save a real Djinni posting page as a test fixture at `apps/api/internal/jobsources/infrastructure/adapters/testdata/djinni_posting.html`, following how `djinni_test.go` loads existing fixtures

**Checkpoint**: Module compiles empty; fixture available for the reader tests

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema, ports and shared plumbing every user story needs

**⚠️ CRITICAL**: No user story work can begin until this phase completes

### Schema and generated access

- [X] T003 Write migration `apps/api/internal/db/migrations/00041_manual_add.sql` exactly as specified in data-model.md — `Subscription.kind` with check constraint and the partial unique index, `SourceRun.subscriptionId` + `trigger` with constraints and index, the `manual` `JobSource` insert, and the deliberately asymmetric Down that keeps the `manual` source row
- [X] T004 [P] Extend `apps/api/internal/db/queries/subscription.sql` — add `kind` to `CreateSubscription`, add `EnsureManualSubscription`, `GetManualSubscription`, `CountJobsForSubscription`, `ManualSubscriptionStats`, and add `AND "kind" = 'crawl'` to `ListEnabledSubscriptions` so the scheduler can never see a manual row
- [X] T005 [P] Extend `apps/api/internal/db/queries/sourcerun.sql` — add `subscriptionId` and `trigger` to `InsertSourceRun`, and add `AND "trigger" <> 'manual'` to `RecentSourceRunsForSource` so manual failures never reach the health threshold
- [X] T006 Run `make sqlc-generate` to regenerate `apps/api/internal/db/sqlcgen/` from T004 and T005, then fix the resulting compile errors at every `InsertSourceRun` and `CreateSubscription` call site

### Extract the shared ingest path

- [X] T007 Move `apps/api/internal/jobsources/interfaces/worker/persist.go` and `dedupe.go` to a new package `apps/api/internal/jobsources/application/ingest/`, exporting `NewPostingBatch`, `PersistBatch`, `DedupeKey` and the `PostingBatch` / `PersistResult` / `InsertedJob` types — mechanical move, no logic change (research D5)
- [X] T008 Move the accompanying tests — `persist_test.go`, `persist_integration_test.go`, `dedupe_test.go`, `merge_test.go`, and the parts of `ports_test.go` they need — into `apps/api/internal/jobsources/application/ingest/` and confirm they pass unchanged
- [X] T009 Update `apps/api/internal/jobsources/interfaces/worker/handler.go` to call the `ingest` package, and to pass `subscriptionId` and `trigger: "scheduled"` when inserting its `SourceRun`

### Adapter port and the manual source

- [X] T010 [P] Add the `PostingReader` interface and the `AsPostingReader` helper to `apps/api/internal/jobsources/domain/adapter.go`, alongside the existing optional interfaces, with the implementor contract from contracts/manual-add.md as doc comments
- [X] T011 [P] Create the no-op `ManualAdapter` in `apps/api/internal/jobsources/infrastructure/adapters/manual.go` — `Key() = "manual"`, kind `manual`, `Search` returning a permanent "not crawlable" error, `HealthCheck` returning healthy, no `PostingReader` (research D4)
- [X] T012 Register `ManualAdapter` in the registry at `apps/api/cmd/server/compose.go:129` and in `apps/api/cmd/seed/main.go:55` so it appears in the sources list and `GetByKey` accepts it

### Subscription kind and its guards

- [X] T013 Add manual-kind support to `apps/api/internal/subscriptions/application/service.go` — `EnsureManualSubscription(ctx, sourceKey)` creating at most one per source with `name: "Manual"` and empty URL, skipping `validateSubscriptionURL` for manual rows
- [X] T014 Add the write guards to the same service — `Create` rejects `kind: "manual"`, `Update` rejects `url`/`cron` changes on a manual row, `Delete` and disable refuse while `CountJobsForSubscription > 0` (FR-015, FR-016)
- [X] T015 Enforce the guards at the HTTP edge in `apps/api/internal/subscriptions/interfaces/http/` — 400 on manual create, 400 on immutable-field update, 409 on delete-with-vacancies, 400 on `POST /subscriptions/{id}/run`, and skip manual rows in `run-all` (contracts/manual-add.md)

### Shared types

- [X] T016 Add `Kind`, `ManualCount`, `LastAddedAt` to `SubscriptionDto`, `SubscriptionID` + `Trigger` to `SourceRunDto`, and the new `ManualAddResultDto` / `ManualVacancyDraftDto` to `apps/api/internal/dto/jobs.go` per data-model.md
- [X] T017 Run `make tygo-generate` and `pnpm --filter @job-finder/shared build` to propagate T016 into `packages/shared/src/generated.ts`

### Foundational tests

- [X] T018 [P] Test the subscription guards in `apps/api/internal/subscriptions/service_test.go` — one manual row per source, implicit creation, rejected manual create, immutable url/cron, refused delete with vacancies
- [X] T019 [P] Test in `apps/api/internal/jobsources/interfaces/worker/scheduler_test.go` that a `kind = 'manual'` subscription is never scheduled and never run by run-all (FR-014)
- [X] T020 Add an integration test asserting migration 00041 is idempotent, that existing rows default to `crawl` / `scheduled`, and that Down leaves the `manual` `JobSource` row and its vacancies intact

**Checkpoint**: Schema, ports, guards and shared types in place — user stories can begin

---

## Phase 3: User Story 1 — Paste a posting URL and get it into the feed (Priority: P1) 🎯 MVP

**Goal**: An operator pastes a Djinni posting URL and gets an ordinary vacancy in the feed
within 30 seconds, with matching, tailoring and tracker all working on it.

**Independent Test**: Paste a Djinni posting URL, submit, confirm a feed card appears with the
real title, company and description, and that its detail view offers the same actions as any
other job.

### Tests for User Story 1

- [X] T021 [P] [US1] Test `MatchesPostingURL` in `apps/api/internal/jobsources/infrastructure/adapters/djinni_test.go` with an accept/reject table — posting paths accepted; `?search_type=basic-search`, `/jobs/`, other hosts and malformed input rejected
- [X] T022 [P] [US1] Test `ReadPosting` against the T002 fixture in the same file, asserting title, company, description, location, remote, salary and `postedAt` are all extracted
- [X] T023 [P] [US1] Test the service outcomes in `apps/api/internal/manualadd/application/service_test.go` — created, duplicate, and each of `invalid_url`, `not_a_posting`, `no_reader`, `unreachable`, `blocked`, `timed_out`, asserting each failure stores no vacancy but writes a run
- [X] T024 [P] [US1] Test the endpoint in `apps/api/internal/manualadd/interfaces/http/manualadd_test.go` — status codes and envelope shape per contracts/manual-add.md
- [X] T025 [US1] Integration test in `apps/api/internal/manualadd/application/` (real Postgres, following `persist_integration_test.go`) — a manual add and a crawl of the same posting yield one vacancy; two concurrent adds of the same URL yield one vacancy; three failed manual adds leave the source healthy

### Implementation for User Story 1

- [X] T026 [US1] Implement `MatchesPostingURL` and `ReadPosting` on `DjinniAdapter` in `apps/api/internal/jobsources/infrastructure/adapters/djinni.go`, extending the selectors `FetchDetail` already uses (`djinni.go:219-250`) with title and company, fetching anonymously through the same retrieval path
- [X] T027 [P] [US1] Define the failure taxonomy in `apps/api/internal/manualadd/domain/errors.go` — the six FR-018 kinds as typed errors carrying operator-facing messages
- [X] T028 [P] [US1] Define the outcome type in `apps/api/internal/manualadd/domain/outcome.go` — `created | duplicate | needs_fill_in | failed`, with the draft payload
- [X] T029 [P] [US1] Define the repository and reader ports in `apps/api/internal/manualadd/domain/ports.go` — what the service needs from sqlc, the adapter registry and the enqueuer, kept narrow so tests can fake them
- [X] T030 [US1] Implement URL resolution in `apps/api/internal/manualadd/application/service.go` — validate scheme before any network call, walk the registry asking each `PostingReader` to claim the URL, distinguish `not_a_posting` (host claimed, wrong shape) from `no_reader` (nobody claimed it)
- [X] T031 [US1] Implement the add flow in the same service — 30 s `context.WithTimeout` covering pacing, fetch and parse; `EnsureManualSubscription`; insert the `SourceRun` with `trigger: "manual"`; persist through `ingest.PersistBatch`; finish the run with found/new or the error
- [X] T032 [US1] Map extraction results to outcomes in the same service — complete → created, existing dedupe key → duplicate carrying the existing job, missing title/company/description → the incomplete branch (returns a plain failure until US3 lands the form)
- [X] T033 [US1] Enqueue downstream work after commit in the same service, mirroring `worker/handler.go:230-241` — enrich when the adapter needs detail, otherwise match and ghost-score — on a background context so FR-003d holds
- [X] T034 [US1] Implement `POST /api/jobs/manual` in `apps/api/internal/manualadd/interfaces/http/manualadd.go` with the status codes and envelope from contracts/manual-add.md
- [X] T035 [US1] Wire the module in `apps/api/cmd/server/compose.go` — a `composeManualAdd` function next to `composeSubscriptions` (`compose.go:357`) — and mount its handler in the router
- [X] T036 [P] [US1] Add `api.jobs.addManual` to `apps/dashboard/src/lib/api.ts` and a `useAddVacancyByUrl` mutation in `apps/dashboard/src/features/feed/hooks.ts` that invalidates the feed query on success
- [X] T037 [US1] Build `apps/dashboard/src/features/feed/AddVacancyForm.tsx` — URL input, submit, per-outcome handling (created → highlight the new card, duplicate → navigate to the existing vacancy, failed → show the reason), disabled while in flight so a double-click cannot double-submit
- [X] T038 [US1] Mount the form in `apps/dashboard/src/features/feed/FeedPage.tsx`
- [X] T039 [P] [US1] Test the form in `apps/dashboard/src/features/feed/AddVacancyForm.test.tsx` — each outcome renders correctly, submit is disabled in flight, failure reasons are shown verbatim

**Checkpoint**: A Djinni URL becomes a feed vacancy end to end. MVP is shippable here.

---

## Phase 4: User Story 2 — See manual additions attributed as "Manual" (Priority: P2)

**Goal**: Manual adds are visibly attributed, filterable across sources, and surfaced at the
top of the feed for 24 hours without lying about their age.

**Independent Test**: Add two vacancies from the same host, open the subscriptions view, and
confirm a Manual subscription under that source shows both, with no cron and no run control.

### Tests for User Story 2

- [X] T040 [P] [US2] Test the feed filter and ordering in `apps/api/internal/jobs/service_test.go` — `onlyManual` returns manual adds across several sources and no crawled ones; a manual add under 24 h sorts first under `sort=score`; the same vacancy does not jump the queue under `sort=date`; the boost expires past 24 h
- [X] T041 [P] [US2] Test that age-sensitive reads see the true `postedAt` — assert a manually added vacancy and the same posting crawled report identical age (SC-006b)
- [X] T042 [P] [US2] Test `ManualSubscriptionStats` returns the correct count and most-recent timestamp derived from run records (FR-017h)

### Implementation for User Story 2

- [X] T043 [US2] Add the `LEFT JOIN "Subscription" s ON s."id" = j."subscriptionId"` and the `only_manual` predicate to `ListJobsByScore`, `ListJobsByDate` and `CountJobs` in `apps/api/internal/db/queries/joblist.sql`
- [X] T044 [US2] Add the 24-hour surfacing term to `ListJobsByScore` only — `ORDER BY (s."kind" = 'manual' AND j."ingestedAt" > now() - interval '24 hours') DESC, mr."score" DESC NULLS LAST, j."ingestedAt" DESC` — leaving `ListJobsByDate` untouched per FR-017d
- [X] T045 [US2] Run `make sqlc-generate` and thread `onlyManual` through the jobs service and its `GET /api/jobs` handler in `apps/api/internal/jobs/`
- [X] T046 [US2] Populate `kind`, `manualCount` and `lastAddedAt` on `SubscriptionDto` in `apps/api/internal/subscriptions/application/service.go`, reading the stats from run records
- [X] T047 [P] [US2] Add `onlyManual` to `JobFilters` in `apps/dashboard/src/lib/api.ts` and its query-param serialisation
- [X] T048 [US2] Add a "Manual" filter control to `apps/dashboard/src/features/feed/FeedPage.tsx`, alongside the existing source and status filters
- [X] T049 [US2] Render manual subscriptions read-only in `apps/dashboard/src/features/sources/SourcesPage.tsx` — show the count and last-added time, hide cron editing, hide the run control, disable delete
- [X] T050 [P] [US2] Test the sources view in `apps/dashboard/src/features/sources/` — a manual subscription renders without cron or run controls and with its count

**Checkpoint**: Manual adds are attributable, filterable, and visible on arrival

---

## Phase 5: User Story 3 — Recover when the page cannot be read (Priority: P3)

**Goal**: An unreadable or unknown host no longer dead-ends — the operator completes the
vacancy by hand from a pre-filled form.

**Independent Test**: Submit a URL whose page yields no usable description, confirm the form
appears carrying the partial data, complete it, save, and find the vacancy in the feed.

### Tests for User Story 3

- [X] T051 [P] [US3] Test the fill-in save path in `apps/api/internal/manualadd/application/fillin_test.go` — required-field rejection naming each missing field, successful save, duplicate detection, and that `postedAt` is never defaulted to now
- [X] T052 [P] [US3] Test that `no_reader` and `incomplete` return 200 with a populated draft once this story is in, and that the draft retains the submitted URL
- [X] T053 [P] [US3] Test the dialog in `apps/dashboard/src/features/feed/ManualFillInDialog.test.tsx` — pre-population from the draft, required-field validation, successful save

### Implementation for User Story 3

- [X] T054 [US3] Implement the fill-in save in `apps/api/internal/manualadd/application/fillin.go` — require title, company and description; default `sourceKey` to `manual` when no reader claimed the host; persist through the same `ingest.PersistBatch` path so the vacancy is indistinguishable from an extracted one
- [X] T055 [US3] Implement `POST /api/jobs/manual/fill-in` in `apps/api/internal/manualadd/interfaces/http/manualadd.go` per contracts/manual-add.md
- [X] T056 [US3] Switch the `no_reader` and `incomplete` outcomes in `apps/api/internal/manualadd/application/service.go` from plain failures to 200 responses carrying a populated draft (FR-019, FR-023)
- [X] T057 [P] [US3] Add `api.jobs.saveManual` and a `useSaveManualVacancy` mutation in `apps/dashboard/src/lib/api.ts` and `apps/dashboard/src/features/feed/hooks.ts`
- [X] T058 [US3] Build `apps/dashboard/src/features/feed/ManualFillInDialog.tsx` — pre-filled from the draft, required-field validation naming what is missing, save
- [X] T059 [US3] Open the dialog from `AddVacancyForm.tsx` on a `needs_fill_in` outcome, carrying the draft through

**Checkpoint**: Every posting anywhere can reach the feed — extracted or hand-filled

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T060 [P] Document the manual-add endpoints and the `PostingReader` port in the Docusaurus site under `docs/`, following how existing HTTP API pages are structured
- [X] T061 [P] Add structured logging to the manual-add path in `apps/api/internal/manualadd/application/service.go`, matching the `slog` shape used by `worker/handler.go` (source, outcome, duration, dedupe key)
- [ ] T062 (NEEDS A HUMAN — live Djinni + running stack) Verify FR-003c by hand — start a Djinni crawl, submit a manual add for the same host mid-crawl, and confirm the add waits for pacing rather than bypassing it, returning `timed_out` if it cannot fit in 30 s
- [X] T063 Run `make sqlc-check` and `make tygo-check` and confirm no generated file is stale or hand-edited (Constitution III)
- [X] T064 Run `make test-lint` — required, this feature spans `apps/api`, `apps/dashboard` and `packages/shared` (Constitution IV)
- [ ] T065 (NEEDS A HUMAN — running stack) Walk the quickstart.md verification checklist end to end and confirm every success criterion SC-001 through SC-008

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: needs Setup — **blocks every user story**
- **US1 (Phase 3)**: needs Foundational. Independently shippable.
- **US2 (Phase 4)**: needs Foundational. Testable without US1 by seeding a manual subscription directly, though in practice US1 supplies the data.
- **US3 (Phase 5)**: needs Foundational. T056 modifies a function US1 creates (T030–T032), so run US3 after US1 rather than concurrently with it.
- **Polish (Phase 6)**: needs whichever stories are being shipped

### Critical path inside Phase 2

T003 → T004/T005 → T006 → everything else. The sqlc regeneration in T006 gates every task
that touches generated code, so it should land early and in one commit.

T007–T009 (the ingest extraction) is independent of T003–T006 and can proceed in parallel with
them, but must complete before T031.

### Within each user story

Tests before implementation. Domain types (T027–T029) before the service (T030–T033) before
the endpoint (T034) before the wiring (T035) before the dashboard (T036–T039).

### Parallel opportunities

- T004, T005 — different query files
- T010, T011 — different Go files, no shared symbols
- T018, T019 — different test files
- T021, T022, T023, T024 — different test files, all before their implementations
- T027, T028, T029 — three small domain files
- T040, T041, T042 — different test files
- T060, T061 — docs and logging, unrelated

---

## Parallel Example: User Story 1

```bash
# Tests first, all in different files:
Task: "Test MatchesPostingURL in adapters/djinni_test.go"
Task: "Test ReadPosting against the fixture in adapters/djinni_test.go"
Task: "Test service outcomes in manualadd/application/service_test.go"
Task: "Test endpoint envelope in manualadd/interfaces/http/manualadd_test.go"

# Then the domain types, three small files:
Task: "Failure taxonomy in manualadd/domain/errors.go"
Task: "Outcome type in manualadd/domain/outcome.go"
Task: "Ports in manualadd/domain/ports.go"
```

---

## Implementation Strategy

### MVP first (US1 only)

1. Phase 1 Setup — 2 tasks
2. Phase 2 Foundational — 18 tasks, the bulk of the schema and plumbing work
3. Phase 3 US1 — 19 tasks
4. **Stop and validate**: paste a Djinni URL, get a vacancy, open it, tailor it
5. Ship

At that point the operator can add Djinni postings by hand and everything downstream works.
Attribution is stored but not yet surfaced, and unreadable hosts return a reason rather than
a form — both acceptable, both by design.

### Incremental delivery

1. Setup + Foundational → foundation ready
2. US1 → test → ship (MVP)
3. US2 → test → ship (attribution visible, feed filter, 24 h surfacing)
4. US3 → test → ship (fill-in recovery, unknown hosts reach the feed)

### Adding more sources later

Each additional host is a follow-up slice of exactly two tasks: implement `PostingReader` on
that adapter, and add a fixture test. No change to the service, the endpoint or the dashboard.
That is the point of the port.

---

## Notes

- 65 tasks: 2 setup, 18 foundational, 19 in US1, 11 in US2, 9 in US3, 6 polish
- [P] marks tasks in different files with no incomplete dependency
- T007–T009 move working code with its tests as the safety net — no logic change, and the
  existing integration tests must stay green through the move
- Commit after each task or logical group; the sqlc and tygo regenerations should each be
  their own commit so a stale generated file is obvious in review
- Every failure path must leave no vacancy behind (FR-021) — assert it, do not assume it
