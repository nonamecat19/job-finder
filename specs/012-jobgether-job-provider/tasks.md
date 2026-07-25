---

description: "Task list for Jobgether Job Source feature"
---

# Tasks: Jobgether Job Source

**Input**: Design documents from `/specs/012-jobgether-job-provider/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/jobgether-adapter.md

**Tests**: Included — plan.md's Testing section and constitution IV require table-driven
adapter tests against fixture HTML, matching `glassdoor_test.go`'s convention.

**Organization**: Tasks are grouped by user story (US1/US2/US3) per spec.md priorities, mirroring
the `GlassdoorAdapter` precedent exactly (structurally identical adapter, wiring, and tests).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths are included in every task

## Path Conventions

Single Go monolith (`apps/api`) + one dashboard edit, per plan.md's Project Structure:

- `apps/api/internal/jobsources/adapters/` — new adapter + tests + fixtures
- `apps/api/internal/subscriptions/`, `apps/api/internal/ingestion/`, `apps/api/internal/enrichment/` — wiring edits
- `apps/api/internal/seed/` — seed data edits
- `apps/api/cmd/server/compose.go` — registration
- `apps/dashboard/src/features/sources/SourcesPage.tsx` — source picker entry

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish fixture and adapter scaffolding shared by all later work

- [X] T001 Create `apps/api/internal/jobsources/adapters/testdata/` fixtures for Jobgether:
      `jobgether_list.html`, `jobgether_list_page2.html`, `jobgether_empty.html`,
      `jobgether_blocked.html`, `jobgether_detail.html` (representative Jobgether search-results,
      second-page, zero-results, blocked/interstitial, and detail-page markup), per
      research.md#R2/#R3 and plan.md's Scale/Scope
- [X] T002 Create the adapter skeleton file `apps/api/internal/jobsources/adapters/jobgether.go`
      with the `JobgetherAdapter` struct (`Scraping *scraping.Service` field, mirroring
      `GlassdoorAdapter`), package-level constants (`jobgetherMaxSubscriptionPages`,
      500ms pacing constant), and stub `Key()`/`Kind()` methods returning `"jobgether"` /
      `dto.SourceKindScrape`

**Checkpoint**: Adapter file and fixtures exist; ready for foundational parsing/scrape logic

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core scrape/parse/pacing machinery every user story's behavior depends on

**⚠️ CRITICAL**: No user story task can be verified end-to-end until this phase is complete

- [X] T003 Implement `jobgetherIsBlockedPage(html string) bool` in
      `apps/api/internal/jobsources/adapters/jobgether.go`, mirroring
      `glassdoorIsBlockedPage`, checking for Jobgether's rate-limit/interstitial markers
      (research.md#R3)
- [X] T004 Implement the Jobgether search-results HTML parser (listing card → field extraction:
      title, company, location, remote indicator, salary text, listing URL, external ID, summary
      description) in `apps/api/internal/jobsources/adapters/jobgether.go`, per research.md#R4's
      field mapping table; card without a title is skipped (mirrors Glassdoor)
- [X] T005 Implement paginated fetch-and-parse loop with 500ms inter-request pacing and a
      `jobgetherMaxSubscriptionPages` cap in `apps/api/internal/jobsources/adapters/jobgether.go`,
      using `scraping.Service.FetchHTML`, per research.md#R7 and FR-010; a later page hitting a
      blocked/unparsable condition ends pagination with whatever was already collected (warning,
      not fatal), while page 1 hitting either condition is fatal, per contracts/jobgether-adapter.md

**Checkpoint**: Core scrape/parse/pacing/blocked-detection machinery exists — user stories can
now be implemented against it

---

## Phase 3: User Story 1 - Discover Jobgether listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: Jobgether listings flow into the job feed, deduplicated and attributed to Jobgether,
via the existing ingestion pipeline — no Jobgether-specific pipeline.

**Independent Test**: Enable the Jobgether source, trigger a run, and confirm new job records
appear in the feed tagged with Jobgether as their origin, each with title, company, location,
remote flag, posting URL, and description; re-running produces zero new duplicates.

### Tests for User Story 1

- [X] T006 [P] [US1] Table-driven test for the search-results parser in
      `apps/api/internal/jobsources/adapters/jobgether_test.go` against
      `testdata/jobgether_list.html` and `testdata/jobgether_list_page2.html`, asserting field
      mapping (title, company, location, remote, salary, URL, ExternalID, description) per
      research.md#R4
- [X] T007 [P] [US1] Test for `jobgetherIsBlockedPage` and the empty-results case in
      `apps/api/internal/jobsources/adapters/jobgether_test.go` against `testdata/jobgether_blocked.html`
      and `testdata/jobgether_empty.html`, asserting the three outcomes (zero results / blocked /
      unparsable) stay distinguishable per FR-011
- [X] T008 [P] [US1] Test for pagination pacing and page-cap behavior in
      `apps/api/internal/jobsources/adapters/jobgether_test.go`, asserting no more than
      `jobgetherMaxSubscriptionPages` fetches occur and partial results are kept when a later page
      is blocked/unparsable (FR-008, FR-010)

### Implementation for User Story 1

- [X] T009 [US1] Implement `Search(ctx, query, config) ([]dto.NormalizedJob, error)` on
      `JobgetherAdapter` in `apps/api/internal/jobsources/adapters/jobgether.go`: require
      `query.SubscriptionURL != ""` (return the "jobgether keyword search not implemented" error
      otherwise, mirroring `GlassdoorAdapter.Search`), drive the paginated fetch/parse loop from
      T005, stamp `SourceKey: "jobgether"` on every result, and stash Jobgether's own
      match-percentage score (when present) into `Raw["jobgetherMatchScore"]` only — never a
      first-class field (FR-012, research.md#R4)
- [X] T010 [US1] Implement `HealthCheck(ctx, config) (bool, error)` on `JobgetherAdapter` in
      `apps/api/internal/jobsources/adapters/jobgether.go`: unauthenticated reachability check
      against the Jobgether homepage/search root, returning `(false, nil)` for
      unreachable/blocked (never a non-nil error for that case), mirroring
      `GlassdoorAdapter.HealthCheck` (contracts/jobgether-adapter.md)
- [X] T011 [US1] Register `JobgetherAdapter` in `apps/api/cmd/server/compose.go`: add a
      `Jobgether adapters.JobgetherAdapter` field, construct `jobgetherAdapter :=
      adapters.JobgetherAdapter{Scraping: p.Scraping}`, add it to the adapter registry list, and
      set `Jobgether: jobgetherAdapter` in the returned struct (mirrors the `glassdoor` wiring at
      compose.go:92/105/116/132)
- [X] T012 [US1] Add `"jobgether"` to the enrich-eligible source-key allowlist in
      `apps/api/internal/ingestion/handler.go`'s `persistIfNew` check (line ~220), alongside
      `djinni`/`dou`/`indeed`/`remoteok`/`glassdoor`/`jobleads`, per
      contracts/jobgether-adapter.md's consumer list
- [X] T013 [US1] Add a `jobgether` entry to `apps/api/internal/seed/testdata.go` (one sample
      seeded job with `SourceKey: "jobgether"`, realistic title/company/salary/URL) mirroring the
      existing Glassdoor seed rows (testdata.go:196/252), so the feed and source filter have
      Jobgether data out of the box
- [X] T014 [US1] Add a `jobgether` row to `apps/api/internal/seed/sourceruns.go` (mirrors
      `{sourceKey: "glassdoor", searchID: "seed:glassdoor:ml-engineer"}` at sourceruns.go:31), so
      the source's last-run summary is populated in seed/demo data

**Checkpoint**: User Story 1 is independently functional — Jobgether listings can be searched,
normalized, deduplicated via the existing pipeline, and seeded/demoed.

---

## Phase 4: User Story 2 - Manage the Jobgether source like any other source (Priority: P2)

**Goal**: Operators can enable/disable, configure, run on demand, health-test, and view run
history for Jobgether from the Sources screen, identically to other sources.

**Independent Test**: From the Sources screen, save a Jobgether subscription, toggle Jobgether
off and on, run a health test, trigger a manual run, and verify run history and counts update.

### Tests for User Story 2

- [X] T015 [P] [US2] Test `validateJobgetherSubscriptionURL` in
      `apps/api/internal/subscriptions/service_test.go` (or the existing subscriptions test
      file): accepts `jobgether.com`/`*.jobgether.com` search-results URLs, rejects other hosts
      and single-job-detail-page paths with a human-readable reason (FR-015, SC-008), mirroring
      the existing `validateGlassdoorSubscriptionURL` test coverage

### Implementation for User Story 2

- [X] T016 [US2] Implement `validateJobgetherSubscriptionURL(rawURL string) error` in
      `apps/api/internal/subscriptions/service.go`: host must be `jobgether.com` or a
      `.jobgether.com` subdomain, path must look like a search-results page (not a single
      job-detail page), mirroring `validateGlassdoorSubscriptionURL` (service.go:150-160)
- [X] T017 [US2] Add a `case "jobgether": return validateJobgetherSubscriptionURL(rawURL)` branch
      to `validateSubscriptionURL` in `apps/api/internal/subscriptions/service.go` (alongside the
      existing `indeed`/`remoteok`/`glassdoor`/`jobleads` cases at service.go:109-115)
- [X] T018 [US2] Add a `jobgether` entry to `apps/api/internal/seed/subscriptions.go` (mirrors
      the Glassdoor seed subscription at subscriptions.go:49-51: `sourceKey: "jobgether"`, a
      descriptive `name`, and a realistic Jobgether search-results `url`)
- [X] T019 [US2] [P] Add a `{ key: 'jobgether', label: 'Jobgether', placeholder: ... }` entry to
      `SUBSCRIPTION_SOURCES` in `apps/dashboard/src/features/sources/SourcesPage.tsx` (alongside
      the `glassdoor` entry at SourcesPage.tsx:220), so Jobgether is selectable in the "New
      Subscription" source picker

**Checkpoint**: User Stories 1 AND 2 both work independently — Jobgether is fully manageable from
the Sources screen and its subscription config is validated at save time.

---

## Phase 5: User Story 3 - Enrich Jobgether listings with full posting detail (Priority: P3)

**Goal**: Jobgether listings are enriched post-ingestion with full description and resolved
posting date, so matching/scoring/generation are grounded in complete posting content.

**Independent Test**: Ingest one Jobgether listing, run enrichment for it, and confirm the
stored description and qualifications are captured in full, with posting date resolved; a
rotated-out listing is marked unavailable without discarding its summary data.

### Tests for User Story 3

- [X] T020 [P] [US3] Test `FetchDetail` in
      `apps/api/internal/jobsources/adapters/jobgether_test.go` against
      `testdata/jobgether_detail.html`: asserts full description and resolved `PostedAt` are
      returned on success, and `Available: false, nil error` (not an error) when the detail page
      shows a rotated-out/expired listing, per contracts/jobgether-adapter.md's `FetchDetail`
      postconditions

### Implementation for User Story 3

- [X] T021 [US3] Define `JobgetherDetailPatch` struct (`Description`, `SalaryRaw *string`,
      `PostedAt *string`, `Available bool`, `Raw map[string]any`) in
      `apps/api/internal/jobsources/adapters/jobgether.go`, matching
      `GlassdoorDetailPatch`'s shape exactly (contracts/jobgether-adapter.md)
- [X] T022 [US3] Implement `FetchDetail(ctx, jobURL, config) (JobgetherDetailPatch, error)` on
      `JobgetherAdapter` in `apps/api/internal/jobsources/adapters/jobgether.go`: fetch the
      listing detail page (unauthenticated, same transport as `Search`), parse full description
      and posting date, capture `jobgetherMatchScore` into `Raw` if present and not already
      captured at list time; return `Available: false, nil` (not an error) when the listing is
      gone (404 or "not found"/expired page), mirroring `GlassdoorAdapter.FetchDetail`
      (research.md#R5)
- [X] T023 [US3] Add a `jobgether adapters.JobgetherAdapter` field and constructor parameter to
      `enrichment.Handler`/`NewHandler` in `apps/api/internal/enrichment/handler.go` (handler.go:31/38/41),
      threaded from `apps/api/cmd/server/compose.go`'s `enrichment.NewHandler(...)` call
      (compose.go:330)
- [X] T024 [US3] Add a `case "jobgether": err = h.enrichJobgether(ctx, payload, uid, job)` branch
      to `enrichment.Handler.ProcessTask`'s `switch job.SourceKey` in
      `apps/api/internal/enrichment/handler.go` (handler.go:117-118)
- [X] T025 [US3] Implement `enrichJobgether(ctx, payload, uid, job)` in
      `apps/api/internal/enrichment/handler.go`, structurally identical to `enrichGlassdoor`
      (handler.go:346-378): apply the enrich-source delay, call `h.jobgether.FetchDetail`, on
      `Available: false` leave existing summary data untouched and log, on success update the
      job's description/salary/postedAt and log completion

**Checkpoint**: All three user stories are independently functional — discovery, management, and
enrichment.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and constitution/spec-required checks that span all stories

- [X] T026 Run `cd apps/api && go test ./internal/jobsources/adapters/... -run Jobgether -v` and
      confirm all Jobgether adapter tests pass against fixtures with no live network calls
      (constitution IV)
- [X] T027 Run `cd apps/api && go test ./...` to confirm the full existing suite (ingestion,
      enrichment, subscriptions, compose wiring) still passes with Jobgether added, and that no
      other source's behavior regressed (SC-007's "no >10% cycle-time regression" is verified
      operationally, not by this test run, but no functional regression should appear here)
- [X] T028 [P] Run the dashboard test suite (`apps/dashboard`) to confirm `SourcesPage` tests, if
      any exercise `SUBSCRIPTION_SOURCES`, still pass with the new `jobgether` entry
- [ ] T029 Execute quickstart.md's manual validation steps 1-10 end-to-end against a local stack
      (source registration, health check, subscription save + rejection, run + feed check, dedupe
      re-run, match-score metadata isolation, blocked/throttled distinguishability, enrichment,
      dashboard visibility, unit tests) to confirm SC-001 through SC-008

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup (T001-T002) completion — BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational (Phase 2) completion; no dependency on
  US2/US3
- **User Story 2 (Phase 4)**: Depends on Foundational (Phase 2) completion; independent of US1's
  implementation tasks, though sequencing after US1 is natural since a subscription needs a
  working `Search` to be useful end-to-end
- **User Story 3 (Phase 5)**: Depends on Foundational (Phase 2) completion and on `Search`
  existing (US1, T009) to have something to enrich; independent of US2
- **Polish (Phase 6)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) — no dependency on other stories
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) — validation logic (T016-T017) is
  independent of US1/US3; seed/dashboard tasks (T018-T019) are additive
- **User Story 3 (P3)**: Can start after Foundational (Phase 2) — `FetchDetail` (T021-T022) is
  independent of US1/US2, but `enrichJobgether` (T023-T025) is only exercisable once US1's
  `Search`/ingestion path (T009, T012) produces jobs to enrich

### Within Each User Story

- Tests before implementation (T006-T008 before T009-T014; T015 before T016-T019; T020 before
  T021-T025)
- Parser/scrape core (Phase 2) before adapter methods that use it (US1)
- Adapter methods before wiring into compose.go/ingestion/enrichment
- Wiring before seed data that exercises it

### Parallel Opportunities

- T001 and T002 (Setup) can run in parallel — fixtures and skeleton file are independent
- Within US1 tests: T006, T007, T008 can run in parallel (same test file, but independent test
  functions — coordinate if editing concurrently)
- T015 (US2 test) can run in parallel with US1 tasks once Foundational is done
- T019 (dashboard SourcesPage edit) can run in parallel with any backend task — different
  language/file entirely
- T020 (US3 test) can run in parallel with US1/US2 tasks once Foundational is done
- T028 (dashboard test run) can run in parallel with T026/T027 (backend test runs)

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Table-driven parser test in apps/api/internal/jobsources/adapters/jobgether_test.go (T006)"
Task: "Blocked/empty detection test in apps/api/internal/jobsources/adapters/jobgether_test.go (T007)"
Task: "Pagination pacing/cap test in apps/api/internal/jobsources/adapters/jobgether_test.go (T008)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T002)
2. Complete Phase 2: Foundational (T003-T005) — CRITICAL, blocks all stories
3. Complete Phase 3: User Story 1 (T006-T014)
4. **STOP and VALIDATE**: Enable Jobgether, run it, confirm listings land in the feed
   deduplicated and attributed correctly (spec's US1 Independent Test)
5. Deploy/demo if ready — Jobgether is discoverable even before US2/US3 land

### Incremental Delivery

1. Setup + Foundational → foundation ready (T001-T005)
2. Add User Story 1 → test independently → deploy/demo (MVP!) (T006-T014)
3. Add User Story 2 → test independently → deploy/demo (T015-T019)
4. Add User Story 3 → test independently → deploy/demo (T020-T025)
5. Polish (T026-T029)

### Parallel Team Strategy

With multiple developers, after Foundational (Phase 2) completes:

- Developer A: User Story 1 (adapter Search/HealthCheck/wiring)
- Developer B: User Story 2 (subscription validation/dashboard picker) — can start once the
  `JobgetherAdapter` type exists (T002), does not need `Search` implemented
- Developer C: User Story 3 (`FetchDetail`/enrichment wiring) — can implement `FetchDetail`
  immediately after T002, but `enrichJobgether`'s end-to-end test needs US1's `Search`/ingestion
  path first

---

## Notes

- [P] tasks = different files or independent concerns, no dependencies
- [Story] label maps task to specific user story for traceability
- This feature adds no new HTTP endpoints, no DB migration, and no new `packages/shared` types —
  every task modifies an existing file or adds a new adapter/test/fixture file, per plan.md's
  Scale/Scope
- Every adapter behavior task (T003-T005, T009-T010, T021-T022) has a Glassdoor-adapter precedent
  cited in its description — deviate only where research.md/contracts explicitly call out a
  Jobgether-specific nuance (the match-percentage score handling, FR-012)
- Verify tests fail before implementing (T006-T008, T015, T020)
- Commit after each task or logical group, per repo convention of small per-feature commits
- Stop at any checkpoint to validate the story independently
