---

description: "Task list for feature 002-indeed-job-source"
---

# Tasks: Indeed Job Source

**Input**: Design documents from `/specs/002-indeed-job-source/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/indeed-adapter.md, quickstart.md

**Tests**: Included — the codebase's existing convention (constitution IV, `dou_test.go`/`workua_test.go`) is fixture-based `go test` coverage for every adapter; these are written per story alongside the code they cover, not as a strict red-green-first TDD gate.

**Organization**: Tasks are grouped by user story (US1 = P1 discover listings, US2 = P2 manage source, US3 = P3 enrich detail) per spec.md.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3
- File paths are exact and relative to repo root

## Path Conventions

Existing Go monorepo, single backend app: `apps/api/internal/...`, `apps/api/cmd/server/...`; one dashboard file under `apps/dashboard/src/...`. No new top-level directories.

---

## Phase 1: Setup

**Purpose**: Recorded fixtures the adapter's tests will parse against — captured/synthesized once, reused by every story's tests.

- [X] T001 [P] Create `apps/api/internal/jobsources/adapters/testdata/indeed_list.html` — a representative Indeed search-results page fixture (job cards: `<h3><a href>` title, company text, location text incl. a "Remote" example, a salary free-text example, and pagination links using `?start=N`), per research.md R3 structure notes
- [X] T002 [P] Create `apps/api/internal/jobsources/adapters/testdata/indeed_list_page2.html` — a second-page fixture (different `start=` offset, at least one card whose title/href differs from page 1, used to verify pagination advances and eventually stops)
- [X] T003 [P] Create `apps/api/internal/jobsources/adapters/testdata/indeed_empty.html` — a valid search-results page with zero job cards (for the "zero results" edge case, FR-011/SC distinguishing "no matches" from "unparseable")
- [X] T004 [P] Create `apps/api/internal/jobsources/adapters/testdata/indeed_detail.html` — a single job-detail page fixture with full description body, a remote/location indicator, and a posted-date element

**Checkpoint**: Fixtures exist; nothing yet depends on live network access for tests.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The adapter must exist, compile, and be registered before any story's behavior (search, management, enrichment) can be exercised end-to-end.

**⚠️ CRITICAL**: No user story task can be verified until this phase is complete.

- [X] T005 Create `apps/api/internal/jobsources/adapters/indeed.go` with package boilerplate, the `IndeedAdapter` struct (`Scraping *scraping.Service` field, matching `DouAdapter`'s shape), `Key() string` returning `"indeed"`, and `Kind() dto.SourceKind` returning `dto.SourceKindScrape`, per contracts/indeed-adapter.md
- [X] T006 In `apps/api/internal/jobsources/adapters/indeed.go`, add package-level constants `indeedMaxSubscriptionPages` (= 50, mirroring `douMaxSubscriptionPages`) and `indeedRequestDelay` (= 500ms, per FR-010), and a defensive remote-keyword regexp (mirroring `douRemoteRe`/`djinniRemoteRe`) matching "remote"/"hybrid"/"work from home"
- [X] T007 In `apps/api/internal/jobsources/adapters/indeed.go`, implement `HealthCheck(ctx, config) (bool, error)` — fetch the Indeed host via `d.Scraping.FetchHTML`, return `(false, nil)` on fetch error (never a non-nil error), `(true, nil)` on a recognizable Indeed response, per contracts/indeed-adapter.md
- [X] T008 In `apps/api/cmd/server/compose.go`, construct `indeedAdapter := adapters.IndeedAdapter{Scraping: p.Scraping}` near the existing `douAdapter`/`djinniAdapter` construction (~line 89) and add `indeedAdapter` to the registry's adapter list (~lines 92-100)
- [X] T009 Verify `go build ./...` succeeds in `apps/api` with the new adapter registered but `Search`/`FetchDetail` still unimplemented stubs returning `not implemented` errors (compile-time checkpoint before story work begins)

**Checkpoint**: `IndeedAdapter` compiles, is registered in the runtime `Registry`, and appears via `GET /api/sources` (registry-derived default row) — ready for story implementation.

---

## Phase 3: User Story 1 - Discover Indeed listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: Enabling Indeed and running it against a saved search URL adds new, deduplicated listings to the job feed, attributed to Indeed.

**Independent Test**: Enable Indeed, save a subscription URL, trigger a run, confirm new `Job` rows appear with `sourceKey="indeed"`, full required fields, and that re-running produces zero new duplicates.

### Tests for User Story 1

- [X] T010 [P] [US1] Write `TestIndeedKey`/`TestIndeedKind` in `apps/api/internal/jobsources/adapters/indeed_test.go` asserting `Key()=="indeed"` and `Kind()==dto.SourceKindScrape`
- [X] T011 [P] [US1] Write `TestParseIndeedCards` in `apps/api/internal/jobsources/adapters/indeed_test.go` loading `testdata/indeed_list.html`, asserting each parsed `dto.NormalizedJob` has `SourceKey=="indeed"`, non-empty `Title`/`URL`, correct `Remote` flag on the "Remote" example card, and a non-nil `SalaryRaw` on the salary example card
- [X] T012 [P] [US1] Write `TestParseIndeedCards_Empty` in `apps/api/internal/jobsources/adapters/indeed_test.go` loading `testdata/indeed_empty.html`, asserting zero jobs and no error (not a failure) — validates the "zero results ≠ error" distinction (FR-011)
- [X] T013 [US1] Write `TestIndeedSearch_Pagination` in `apps/api/internal/jobsources/adapters/indeed_test.go` using an `httptest.Server` serving `testdata/indeed_list.html` then `testdata/indeed_list_page2.html` then an empty page, asserting `Search` follows `start=` increments, stops on an empty page, and returns the union of both pages' jobs
- [X] T014 [US1] Write `TestIndeedSearch_NoSubscriptionURL` in `apps/api/internal/jobsources/adapters/indeed_test.go` asserting `Search` with an empty `query.SubscriptionURL` returns a non-nil error (keyword search out of scope, FR-015)

### Implementation for User Story 1

- [X] T015 [US1] In `apps/api/internal/jobsources/adapters/indeed.go`, implement `parseIndeedCards(doc *goquery.Document) []dto.NormalizedJob` per research.md R3: select `<h3>` title anchors, extract `Title`/`URL` (resolve relative to absolute), nearby company/location/salary text with defensive multi-selector fallback, `Remote` via the T006 regexp against location+snippet text, best-effort `ExternalID` from the URL's `jk=`/path segment, `PostedAt` left nil at list-level (filled by enrichment)
- [X] T016 [US1] In `apps/api/internal/jobsources/adapters/indeed.go`, implement pagination helper that takes a base subscription URL, increments its `start` query parameter by 10 per page, stops when a page yields zero new cards, the page cap (T006 constant) is reached, or the first card repeats the previous page's first card (loop guard, mirroring `djinniMaxSubscriptionPages`/`seenFirstHref`), sleeping `indeedRequestDelay` between requests (FR-010)
- [X] T017 [US1] In `apps/api/internal/jobsources/adapters/indeed.go`, implement `Search(ctx, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)`: if `query.SubscriptionURL == ""` return the keyword-not-implemented error (T014); otherwise call the T016 paginator using `d.Scraping.FetchHTML` + `parseIndeedCards`, returning partial results with `nil` error if a later page fails (only page 1 failing is fatal), per contracts/indeed-adapter.md
- [X] T018 [US1] Run `go test ./apps/api/internal/jobsources/adapters/... -run Indeed -v` and fix until T010–T014 pass

**Checkpoint**: User Story 1 is independently functional — `Search` against a real or fixture subscription URL returns normalized, deduplicatable jobs; existing `ingestion.Handler.persistIfNew`/dedupe/matching/ghost-score paths need no change to consume them (dedupe key and downstream enqueue already operate on any `dto.NormalizedJob`, per data-model.md).

---

## Phase 4: User Story 2 - Manage the Indeed source like any other source (Priority: P2)

**Goal**: Operators enable/disable Indeed, run a health test, trigger a manual run, and manage its subscription URL from the Sources screen, exactly like DOU/Djinni.

**Independent Test**: From the Sources screen (or its REST endpoints), toggle Indeed, run `/test`, save/reject a subscription URL, and confirm run history updates — none of it touching other sources.

### Tests for User Story 2

- [X] T019 [P] [US2] Write `TestIndeedHealthCheck` in `apps/api/internal/jobsources/adapters/indeed_test.go` using an `httptest.Server`, asserting `(true, nil)` on a 200 response and `(false, nil)` (not an error) on a connection failure
- [X] T020 [P] [US2] Write `TestValidateIndeedSubscriptionURL` in `apps/api/internal/subscriptions/service_test.go` asserting a `indeed.com`/`*.indeed.com` job-search URL is accepted and a non-Indeed URL (e.g. `https://example.com/x`) is rejected with an error, for `sourceKey=="indeed"` (FR-016)

### Implementation for User Story 2

- [X] T021 [US2] In `apps/api/internal/subscriptions/service.go`, add a source-key-aware URL validation step inside `Create` (and `Update` where the URL changes): for `sourceKey=="indeed"`, parse the URL and reject (return an error, no insert) unless its host is `indeed.com` or ends in `.indeed.com` and its path looks like a job-search listing (not an individual job-posting path) — implements data-model.md's Subscription validation rule
- [X] T022 [US2] In `apps/dashboard/src/features/sources/SourcesPage.tsx`, add `{ key: 'indeed', label: 'Indeed', placeholder: 'https://www.indeed.com/jobs?q=golang&l=remote' }` to the `SUBSCRIPTION_SOURCES` array (~line 203-204) so Indeed appears in the "New Subscription" source picker
- [X] T023 [US2] Run `go test ./apps/api/internal/jobsources/adapters/... ./apps/api/internal/subscriptions/... -run Indeed -v` and fix until T019–T020 pass

**Checkpoint**: User Stories 1 AND 2 both work independently — Indeed is fully manageable via the existing `/api/sources`, `/api/sources/{key}/test`, and `/api/subscriptions` endpoints, and visible/selectable in the dashboard.

---

## Phase 5: User Story 3 - Enrich Indeed listings with full posting detail (Priority: P3)

**Goal**: After ingestion, an enrichment pass fills in the full description, remote status, and posted date for each Indeed listing.

**Independent Test**: Ingest one Indeed listing (list-level data only), run the enrich task for it, confirm its stored `description` grows to the full posting text and `postedAt`/`remote` resolve where the detail page publishes them; confirm a gone-404 detail page leaves the existing summary data untouched rather than erroring the job.

### Tests for User Story 3

- [X] T024 [P] [US3] Write `TestIndeedFetchDetail` in `apps/api/internal/jobsources/adapters/indeed_test.go` loading `testdata/indeed_detail.html` via an `httptest.Server`, asserting `IndeedDetailPatch.Description` is non-empty and longer than a typical list-card snippet, `Remote`/`PostedAt` are resolved per the fixture's content
- [X] T025 [P] [US3] Write `TestIndeedFetchDetail_NotFound` in `apps/api/internal/jobsources/adapters/indeed_test.go` using an `httptest.Server` returning 404, asserting `FetchDetail` returns a non-nil error (caller decides to preserve existing summary data, not this method)
- [X] T026 [US3] Write `TestEnrichIndeed` in `apps/api/internal/enrichment/handler_test.go` (create if it does not already cover this path) asserting `ProcessTask` with `job.SourceKey=="indeed"` calls the Indeed adapter's `FetchDetail` and persists via `UpdateJobDetail`, and that a `FetchDetail` error logs a warning and returns `nil` (job's existing fields untouched) rather than propagating the error to asynq

### Implementation for User Story 3

- [X] T027 [US3] In `apps/api/internal/jobsources/adapters/indeed.go`, define `IndeedDetailPatch struct { Description string; SalaryRaw *string; Location *string; Remote bool; PostedAt *string; Raw map[string]string }` per contracts/indeed-adapter.md
- [X] T028 [US3] In `apps/api/internal/jobsources/adapters/indeed.go`, implement `FetchDetail(ctx, jobURL string, config map[string]any) (IndeedDetailPatch, error)`: fetch the job URL via `d.Scraping.FetchHTML`, parse full description/remote/posted-date with defensive selectors (mirroring `DjinniAdapter.FetchDetail`/`DouAdapter.FetchDetail`), return a non-nil error on fetch failure (404/gone) so the caller can skip the update
- [X] T029 [US3] In `apps/api/internal/enrichment/handler.go`, add an `indeed adapters.IndeedAdapter` field to `Handler` and a matching parameter to `NewHandler(...)` (after the existing `workua` param)
- [X] T030 [US3] In `apps/api/internal/enrichment/handler.go`, add `case "indeed": err = h.enrichIndeed(ctx, payload, uid, job); return err` to the `switch job.SourceKey` in `ProcessTask` (~line 97-109)
- [X] T031 [US3] In `apps/api/internal/enrichment/handler.go`, implement `enrichIndeed(ctx, payload, uid, job)` mirroring `enrichDOU`: apply `h.delayFor("indeed")` pacing, call `h.indeed.FetchDetail`, on error log a warning and `return nil` (preserve summary data), on success `UpdateJobDetail` and `enqueueMatch`/`enqueueSalaryInfer`
- [X] T032 [US3] In `apps/api/internal/ingestion/handler.go`, extend the enrich-eligibility check in `persistIfNew` (~line 214) from `if j.SourceKey == "djinni" || j.SourceKey == "dou"` to also include `|| j.SourceKey == "indeed"` so newly-ingested Indeed jobs are auto-enqueued for detail enrichment
- [X] T033 [US3] In `apps/api/cmd/server/compose.go`, thread `sources.Indeed` into the `enrichment.NewHandler(...)` call (~line 299), matching the pattern of `sources.Djinni, sources.Dou, sources.Workua`; add an `Indeed adapters.IndeedAdapter` field alongside `Djinni`/`Dou`/`Workua` in the sources-provider struct (~lines 79-109) if not already exposed there
- [X] T034 [US3] Run `go test ./apps/api/internal/jobsources/adapters/... ./apps/api/internal/enrichment/... -run Indeed -v` and fix until T024–T026 pass

**Checkpoint**: All three user stories are independently functional. Indeed listings ingest, are manageable, and get enriched with full detail — matching DOU/Djinni parity end-to-end.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation across all stories; no new functional surface.

- [X] T035 Run `go build ./...` and `go vet ./...` in `apps/api` to confirm no unused params/dead code left from the phased wiring (e.g. stub errors from T009)
- [X] T036 Run `make test-lint` (all three suites — constitution IV) to confirm no cross-app regression from the `SourcesPage.tsx` and Go changes
- [X] T037 Walk through `specs/002-indeed-job-source/quickstart.md` end-to-end against a running local stack (`make up`), recording any deviation from the documented `curl` responses
- [X] T038 [P] Update `apps/api/internal/seed/subscriptions.go` and `apps/api/internal/seed/testdata.go` with one Indeed seed subscription and one or two seed `Job` rows (`sourceKey: "indeed"`), mirroring the existing djinni/dou seed entries, so local dev/demo data includes Indeed (plan.md Assumptions: "seed/demo data... follow the same conventions as existing sources")

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — fixture files can be written immediately, in parallel (T001-T004 touch different files)
- **Foundational (Phase 2)**: Depends on Setup only in that T005-T007 will eventually be tested against Phase 1's fixtures; T005-T009 themselves have no fixture dependency and could start in parallel with Phase 1, but must all complete — BLOCKS all user stories (registry must be wired before any story is testable end-to-end)
- **User Story 1 (Phase 3)**: Depends on Foundational (Phase 2) only
- **User Story 2 (Phase 4)**: Depends on Foundational (Phase 2) only — does NOT depend on US1's `Search`/pagination implementation (T015-T017); `HealthCheck` (T007) and subscription validation (T021) are independent of card-parsing logic
- **User Story 3 (Phase 5)**: Depends on Foundational (Phase 2); T032/T033 touch the same ingestion/compose wiring conceptually exercised in Phase 2/US1 but are additive lines, not conflicting edits — can start once Phase 2 is done, though T026 (integration test) is most meaningful once US1's `Search`+persist path (Phase 3) is also in place
- **Polish (Phase 6)**: Depends on all three stories being complete

### User Story Dependencies

- **US1 (P1)**: No dependency on US2/US3 — a subscription can be created via direct SQL/API call for its own test without US2's validation task, and detail stays list-level (no enrichment) without US3
- **US2 (P2)**: Independent of US1's parsing logic; only needs the Foundational registration (Phase 2) and the adapter's `HealthCheck` (T007)
- **US3 (P3)**: Independent of US2; benefits from US1 existing (something to enrich) but its own tests (T024-T026) can run against a directly-constructed `Job` row without going through `Search`

### Within Each User Story

- Tests before their corresponding implementation task where both are listed (e.g., T010-T014 before T015-T017)
- `parseIndeedCards` (T015) before pagination (T016) before `Search` (T017)
- `IndeedDetailPatch` (T027) before `FetchDetail` (T028) before the enrichment handler wiring (T029-T031)

### Parallel Opportunities

- T001-T004 (all fixture files) — fully parallel
- T010-T012 (independent test functions, same file but non-overlapping — still safe to author in parallel and merge) — parallel-eligible in spirit; if strictly enforcing "different files" for [P], treat as sequential edits to one file instead
- T019-T020 — different files ( `indeed_test.go` vs `subscriptions/service_test.go`), fully parallel
- T024-T025 — same file, sequential in practice despite conceptual independence
- T038 — independent of T035-T037, can run in parallel

---

## Parallel Example: Phase 1 (Setup)

```bash
Task: "Create testdata/indeed_list.html fixture"
Task: "Create testdata/indeed_list_page2.html fixture"
Task: "Create testdata/indeed_empty.html fixture"
Task: "Create testdata/indeed_detail.html fixture"
```

## Parallel Example: Phase 4 (User Story 2)

```bash
Task: "Write TestIndeedHealthCheck in apps/api/internal/jobsources/adapters/indeed_test.go"
Task: "Write TestValidateIndeedSubscriptionURL in apps/api/internal/subscriptions/service_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (fixtures) + Phase 2 (adapter skeleton + registration)
2. Complete Phase 3 (US1: `Search` + pagination + parsing)
3. **STOP and VALIDATE**: run `go test -run Indeed`, then quickstart.md steps 1, 3 (subscription create), 4, 5 manually against a real or locally-stubbed Indeed URL
4. This alone delivers the core value: Indeed listings flowing into the job feed, deduplicated

### Incremental Delivery

1. Setup + Foundational → adapter registered, compiles, appears in `/api/sources`
2. Add US1 → Indeed listings ingest and dedupe → demoable MVP
3. Add US2 → operators can manage/validate/health-check it from the Sources screen
4. Add US3 → listings get full-detail enrichment
5. Polish → seed data, quickstart walkthrough, lint/build across all three language suites

### Parallel Team Strategy

With multiple developers, once Phase 2 (Foundational) lands:

- Developer A: Phase 3 (US1 — parsing/pagination/Search)
- Developer B: Phase 4 (US2 — HealthCheck already in Foundational; validation + dashboard entry)
- Developer C: Phase 5 (US3 — FetchDetail + enrichment wiring)

All three converge on the same `indeed.go` file for different methods, so in practice this is best run as a fast sequence (Foundational → US1 → US2 → US3) by one implementer, or coordinated closely if split, to avoid merge conflicts within `indeed.go`/`indeed_test.go`.

---

## Notes

- [P] tasks = different files, no dependencies — verify before parallelizing; several nominally-parallel test tasks above share one file (`indeed_test.go`) and are noted as sequential-in-practice
- [Story] label maps task to specific user story for traceability
- Every adapter method (`Search`, `HealthCheck`, `FetchDetail`) already has a direct precedent in `dou.go`/`djinni.go` — implementation tasks should read those files first rather than designing selectors from scratch
- No DB migration in this feature — do not add one
- Commit after each task or logical group (constitution: small, per-feature commits — see project convention)
- Stop at any checkpoint to validate story independently
