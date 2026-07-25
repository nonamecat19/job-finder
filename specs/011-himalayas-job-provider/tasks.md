---

description: "Task list for Himalayas Job Source"
---

# Tasks: Himalayas Job Source

**Input**: Design documents from `/specs/011-himalayas-job-provider/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/himalayas-adapter.md, quickstart.md

**Tests**: Included — the plan's Testing section and Constitution Principle IV (Test Discipline
Per Language, Enforced at the Boundary) require fixture-based `go test` coverage for the new
adapter, mirroring `remoteok_test.go`/`arbeitnow_test.go`.

**Organization**: Tasks are grouped by user story (spec.md P1/P2/P3) to enable independent
implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- File paths are exact and relative to the repository root

## Path Conventions

This is the existing `apps/api` (Go) + `apps/dashboard` (React) monorepo — see plan.md's Project
Structure section. All new/edited files live under `apps/api/internal/jobsources/adapters/`,
`apps/api/internal/subscriptions/`, `apps/api/cmd/server/`, and one entry in
`apps/dashboard/src/features/sources/SourcesPage.tsx`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Recorded fixtures the whole test suite depends on

- [X] T001 [P] Create recorded Himalayas `/jobs/api` list-response fixture at `apps/api/internal/jobsources/adapters/testdata/himalayas_list.json` — a small paginated page (`offset`, `limit`, `totalCount`, `jobs` array) with 2-3 realistic job records covering: a job with `categories`/`parentCategories` matching a test category slug, a job with non-empty `timezoneRestrictions`, a job with `locationRestrictions` empty and non-empty, a job with `minSalary`/`maxSalary`/`currency`/`salaryPeriod` present, and a job with those salary fields absent/zero — per research.md#r5-job-field-mapping
- [X] T002 [P] Create recorded Himalayas zero-results-shape fixture at `apps/api/internal/jobsources/adapters/testdata/himalayas_empty.json` — valid decodable shape (`offset`, `limit`, `totalCount: 0`, `jobs: []`) per the spec's "zero results" edge case

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core adapter types and helpers that both `Search` (US1) and `HealthCheck`/validation
(US2) depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T003 Create `apps/api/internal/jobsources/adapters/himalayas.go` with package doc comment (mirroring `remoteok.go`'s header — public unauthenticated JSON feed, no login, RemoteOK/Arbeitnow hybrid pattern per plan.md Summary), the `HimalayasAdapter` struct (`Scraping *scraping.Service`, `APIURL string`), `Key() string` returning `"himalayas"`, `Kind() dto.SourceKind` returning `dto.SourceKindAPI`, an `apiURL()` helper honoring the `APIURL` override (mirroring `RemoteOKAdapter.apiURL()`), and the `himalayasAPIURL`/`himalayasUserAgent`/`himalayasRequestDelay` (500ms)/`himalayasMaxSubscriptionPages` constants (FR-010, FR-017)
- [X] T004 Add raw JSON decode types to `apps/api/internal/jobsources/adapters/himalayas.go`: `himalayasResponse` (`offset`, `limit`, `totalCount int`, `jobs []himalayasJob`) and `himalayasJob` (`guid`, `title`, `companyName`, `categories []string`, `parentCategories []string`, `timezoneRestrictions []string`, `locationRestrictions []string`, `minSalary`/`maxSalary int`, `currency`/`salaryPeriod string`, `pubDate int64`, `description string`, `seniority`/`employmentType string`) per research.md#r5-job-field-mapping and data-model.md's Normalized Job Listing section
- [X] T005 Implement `himalayasJobFromRaw(raw himalayasJob) dto.NormalizedJob` in `apps/api/internal/jobsources/adapters/himalayas.go`: `SourceKey: "himalayas"`, `ExternalID`/`URL` both from `guid`, `Remote: true` unconditionally, `Location` from `locationRestrictions` (empty slice → `nil`), `SalaryRaw` formatted from `minSalary`/`maxSalary`/`currency`/`salaryPeriod` (all-zero/absent → `nil`), `PostedAt` converted from the `pubDate` Unix-seconds integer to RFC3339 UTC, `Description` copied in full (no truncation — data-model.md R6: no separate summary/detail split), and `Raw` carrying `timezoneRestrictions` (folded into descriptive text per the "timezone band" edge case), `categories`, `parentCategories`, `seniority`, `employmentType`
- [X] T006 Implement `himalayasSubscriptionFilter` parsing in `apps/api/internal/jobsources/adapters/himalayas.go`: a helper that parses a Himalayas `/jobs?categories=<slug>[,<slug>...][&timezones=<a,b>]` search-page URL into a category-slug set and optional timezone set, returning a distinguishable error when `categories` is empty or the URL doesn't parse (contracts/himalayas-adapter.md's Search precondition)

**Checkpoint**: Foundation ready — adapter shape, JSON decoding, field mapping, and subscription
URL parsing exist; `Search`, `HealthCheck`, and subscription validation can now be built on top

---

## Phase 3: User Story 1 - Discover Himalayas listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: Himalayas listings flow into the job feed via `Search`, attributed to Himalayas and
deduplicated across runs, using the existing pipeline unchanged (FR-001–FR-004, FR-010–FR-012)

**Independent Test**: Enable the Himalayas source, trigger a run against a saved subscription, and
confirm new job records appear in the feed tagged with Himalayas as their origin, each with title,
company, location, remote flag, posting URL, and description; re-running adds zero duplicates.

### Tests for User Story 1

- [X] T007 [P] [US1] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: `Search` against an `httptest.Server` serving `testdata/himalayas_list.json` returns `[]dto.NormalizedJob` with `SourceKey: "himalayas"`, `Remote: true` on every element, correct field mapping (title, company, URL/ExternalID from `guid`, description, salary, location, postedAt), and keeps only jobs whose `categories`/`parentCategories` match the subscription's category slug
- [X] T008 [P] [US1] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: `Search` against `testdata/himalayas_empty.json` returns an empty slice and a nil error (zero-results is not an error, FR-011); a separate case where the server returns invalid/unparseable JSON returns a non-nil, distinguishable error (response-shape-changed, FR-011)
- [X] T009 [P] [US1] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: `Search` with an empty `query.SubscriptionURL` returns the keyword-search-not-implemented error (mirroring `RemoteOKAdapter.Search`'s message, FR-014)
- [X] T010 [P] [US1] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: `Search` pages through multiple `httptest.Server` responses (`offset=0`, `offset=20`, ...) up to `himalayasMaxSubscriptionPages`, stops early once a page's `offset >= totalCount`, and waits at least `himalayasRequestDelay` between requests (FR-010)

### Implementation for User Story 1

- [X] T011 [US1] Implement `Search(ctx, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)` in `apps/api/internal/jobsources/adapters/himalayas.go`: precondition checks (empty `SubscriptionURL` → keyword-search error per T009; unparseable/empty-categories URL → distinguishable error via T006's helper), then page through `apiURL()?limit=20&offset=N` via `d.Scraping.FetchHTML` with the `himalayasUserAgent` header, decoding each page with `himalayasResponse`, filtering each page's jobs locally against the parsed category/timezone filter, mapping matches via `himalayasJobFromRaw`, pacing requests at `himalayasRequestDelay`, capped at `himalayasMaxSubscriptionPages`, stopping early once `offset >= totalCount` (depends on T003-T006)
- [X] T012 [US1] Register `HimalayasAdapter{Scraping: p.Scraping}` in `apps/api/cmd/server/compose.go`'s adapter set and registry list, mirroring the existing `remoteokAdapter` construction and registration (lines ~91-131)

**Checkpoint**: At this point, User Story 1 is fully functional — Himalayas listings can be
searched, normalized, deduplicated (via existing `ExternalID`-based persistence, unchanged), and
appear in the feed independently of Sources-screen management (US2) or enrichment (US3)

---

## Phase 4: User Story 2 - Manage the Himalayas source like any other source (Priority: P2)

**Goal**: Operators can enable/disable Himalayas, save/reject subscriptions with a stated reason,
run a health check, and see it in the dashboard's source list and subscription picker
(FR-005, FR-006, FR-014, FR-015, FR-016)

**Independent Test**: From the Sources screen, save a Himalayas subscription (accepted) and a
malformed one (rejected with a reason), toggle Himalayas off/on, and trigger a health test — all
without touching other sources' behavior.

### Tests for User Story 2

- [X] T013 [P] [US2] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: `HealthCheck` returns `(true, nil)` when the `httptest.Server` response decodes with a non-nil `jobs` field (including an empty page), and `(false, nil)` — never a non-nil error — when the server is unreachable or returns unparseable JSON
- [X] T014 [P] [US2] Unit test in `apps/api/internal/subscriptions/service_test.go`: `validateSubscriptionURL` accepts `https://himalayas.app/jobs?categories=Backend-Engineering` and a `*.himalayas.app` subdomain with `/jobs/...` path plus non-empty `categories`; rejects a non-Himalayas host, a Himalayas URL with the wrong path, and a Himalayas `/jobs` URL missing the `categories` query parameter — each rejection carrying a human-readable reason (FR-015, SC-008)

### Implementation for User Story 2

- [X] T015 [US2] Implement `HealthCheck(ctx, config map[string]any) (bool, error)` in `apps/api/internal/jobsources/adapters/himalayas.go`: fetch `apiURL()?limit=1&offset=0` via `d.Scraping.FetchHTML`, return `(false, nil)` on fetch/decode failure, `(true, nil)` when the response decodes and `jobs` is non-nil (mirroring `RemoteOKAdapter.HealthCheck`'s convention exactly, contracts/himalayas-adapter.md)
- [X] T016 [US2] Add a `case "himalayas":` to `validateSubscriptionURL` in `apps/api/internal/subscriptions/service.go` (near the existing `case "remoteok":` at line ~111): accept host `himalayas.app` or a `.himalayas.app` subdomain, path `/jobs` or a `/jobs/...` prefix, and require a non-empty `categories` query parameter; reject otherwise with a stated reason (data-model.md's Source Subscription section)
- [X] T017 [US2] Add `{ key: 'himalayas', label: 'Himalayas', placeholder: 'https://himalayas.app/jobs?categories=Backend-Engineering' }` to `SUBSCRIPTION_SOURCES` in `apps/dashboard/src/features/sources/SourcesPage.tsx` (near the existing `remoteok` entry at line ~219)

**Checkpoint**: At this point, User Stories 1 AND 2 both work independently — Himalayas is
runnable, health-checkable, manageable from the dashboard, and rejects invalid subscriptions at
save time

---

## Phase 5: User Story 3 - Enrich Himalayas listings with full posting detail (Priority: P3)

**Goal**: Confirm the full description, qualifications text, and resolved posting date are present
from the moment a Himalayas job is ingested — since research.md R6 establishes Himalayas's feed
has no summary/detail split, this story is satisfied by `Search`'s existing field mapping (T005)
with **no** new `FetchDetail` method, no enrichment-handler wiring, and no ingestion
enrich-eligibility change (FR-009; contracts/himalayas-adapter.md; data-model.md)

**Independent Test**: Ingest one Himalayas listing via `Search` and confirm the stored posting
already contains the full description text, folded-in timezone-restriction text, and a resolved
`PostedAt` — with no separate enrichment call required.

### Tests for User Story 3

- [X] T018 [P] [US3] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: a normalized job's `Description` field equals the fixture's full `description` text verbatim (not a truncated teaser), and `Raw["timezoneRestrictions"]` reflects the fixture's non-empty timezone-restriction case (spec's "listing restricts applicants to a specific timezone band" edge case)
- [X] T019 [P] [US3] Unit test in `apps/api/internal/jobsources/adapters/himalayas_test.go`: a normalized job's `PostedAt` is correctly converted from the fixture's Unix-seconds `pubDate` integer to an RFC3339 UTC string

### Implementation for User Story 3

- [X] T020 [US3] Add a doc comment to `apps/api/internal/jobsources/adapters/himalayas.go` (near the `HimalayasAdapter` type, alongside T003's header) explicitly recording that Himalayas has no `FetchDetail` method and is intentionally absent from `ingestion.Handler`'s enrich-eligible source list (`apps/api/internal/ingestion/handler.go` line ~220) and `enrichment.Handler`'s dispatch `switch job.SourceKey` (`apps/api/internal/enrichment/handler.go` line ~101) — both files are read-only confirmation for this task, no code change to either (research.md R6)

**Checkpoint**: All user stories are independently functional — Himalayas listings are complete at
ingestion, with no pending "detail not yet fetched" state (data-model.md's State transitions)

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification across all three stories

- [X] T021 [P] Review `apps/api/internal/jobsources/adapters/himalayas.go` for parity with `RemoteOKAdapter`'s defensive-decoding style (no panics on malformed upstream fields) and add any missing doc comments
- [X] T022 Run `cd apps/api && go test ./internal/jobsources/adapters/... -run Himalayas -v` and fix any failures until all Phase 3-5 tests pass
- [X] T023 Run `cd apps/api && go test ./internal/subscriptions/...` to confirm T014's `validateSubscriptionURL` case passes alongside existing source cases
- [X] T024 Run `cd apps/api && go test ./...` and `cd apps/dashboard && pnpm test` (or the project's dashboard test command) to confirm no regressions to other sources or the `SourcesPage` test suite (SC-007)
- [ ] T025 Execute quickstart.md steps 1-9 end-to-end against the local stack (`make up`/`make dev`) to validate registration, health check, subscription save/reject, run + dedupe, and dashboard visibility (not run in this worktree — requires a live local stack)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup (fixtures feed the tests written alongside
  Phase 2/3) — BLOCKS all user stories
- **User Stories (Phase 3-5)**: All depend on Foundational phase completion
  - US1 and US2 are independent of each other and can proceed in parallel
  - US3 depends only on US1's `himalayasJobFromRaw` mapping (T005, already in Foundational) — no
    new production code, so it can proceed alongside US1/US2 once Foundational is done
- **Polish (Phase 6)**: Depends on all three user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) — no dependency on US2/US3
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) — independent of US1's `Search`
  implementation (uses the same adapter file but different methods/functions); the dashboard edit
  (T017) has no backend dependency at all
- **User Story 3 (P3)**: Can start after Foundational (Phase 2) — purely additive tests plus a
  doc comment; no implementation dependency on US1/US2 code, though it exercises the same
  `himalayasJobFromRaw` function US1 relies on

### Within Each User Story

- Tests before implementation (write T007-T010 before T011-T012; T013-T014 before T015-T017;
  T018-T019 before T020)
- Foundational types/helpers (T003-T006) before any Search/HealthCheck/validation logic
- Story complete before moving to Polish

### Parallel Opportunities

- T001 and T002 (fixtures) in parallel
- All US1 test tasks (T007-T010) in parallel once Foundational is done
- T013 and T014 (US2 tests, different files) in parallel
- T018 and T019 (US3 tests, same file but independent assertions) in parallel
- Once Foundational (Phase 2) completes, US1, US2, and US3 can all proceed in parallel by
  different contributors since they touch mostly-disjoint functions within `himalayas.go` plus
  entirely separate files (`compose.go`, `subscriptions/service.go`, `SourcesPage.tsx`)

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Unit test: Search maps and filters fixture jobs correctly in apps/api/internal/jobsources/adapters/himalayas_test.go"
Task: "Unit test: Search zero-results vs unparseable-response distinction in apps/api/internal/jobsources/adapters/himalayas_test.go"
Task: "Unit test: Search empty SubscriptionURL error in apps/api/internal/jobsources/adapters/himalayas_test.go"
Task: "Unit test: Search pagination, cap, and pacing in apps/api/internal/jobsources/adapters/himalayas_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (fixtures)
2. Complete Phase 2: Foundational (adapter shape, JSON types, field mapping, URL parsing) —
   CRITICAL, blocks all stories
3. Complete Phase 3: User Story 1 (Search + compose.go registration)
4. **STOP and VALIDATE**: `go test ./internal/jobsources/adapters/... -run Himalayas -v`, then
   confirm listings land in the feed via a manual run
5. Deploy/demo if ready — Himalayas listings now flow into the feed even before Sources-screen
   management (US2) or the enrichment posture confirmation (US3) are done

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. Add User Story 1 → test independently → deploy/demo (MVP: listings flow in)
3. Add User Story 2 → test independently → deploy/demo (operators can manage it)
4. Add User Story 3 → test independently → deploy/demo (confirms no gap versus other
   enrich-eligible sources)
5. Phase 6: Polish — full regression pass, quickstart validation

### Parallel Team Strategy

With multiple contributors:

1. Complete Setup + Foundational together (single small file, likely one contributor)
2. Once Foundational is done:
   - Contributor A: User Story 1 (`Search` + `compose.go`)
   - Contributor B: User Story 2 (`HealthCheck` + `subscriptions/service.go` +
     `SourcesPage.tsx`)
   - Contributor C: User Story 3 (tests + doc comment only)
3. Stories complete and integrate independently since they touch disjoint functions/files

---

## Notes

- [P] tasks = different files or independent assertions within the same test file, no
  dependencies between them
- [Story] label maps task to specific user story (US1/US2/US3) for traceability
- Verify tests fail before implementing (T007-T010, T013-T014, T018-T019 should fail against a
  stub/missing implementation first)
- No new tables, no new migration, no new config/env vars (plan.md's Storage/Constraints sections)
- No `FetchDetail` method and no enrichment-handler wiring for Himalayas — this is a deliberate
  design decision (research.md R6), not a gap to fill; US3 is satisfied entirely by US1's field
  mapping plus confirmation tests/comments
- Commit after each task or logical group, per this project's small-per-feature-commit convention
- Stop at any checkpoint to validate a story independently
