---

description: "Task list for feature 003-remoteok-job-provider"
---

# Tasks: RemoteOK Job Source

**Input**: Design documents from `/specs/003-remoteok-job-provider/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/remoteok-adapter.md, quickstart.md

**Tests**: Included — the codebase's existing convention (constitution IV,
`indeed_test.go`/`dou_test.go`) is fixture-based `go test` coverage for every adapter;
these are written per story alongside the code they cover, not as a strict red-green-first
TDD gate.

**Organization**: Tasks are grouped by user story (US1 = P1 discover listings, US2 = P2
manage source, US3 = P3 enrich detail) per spec.md.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 / US2 / US3
- File paths are exact and relative to repo root

## Path Conventions

Existing Go monorepo, single backend app: `apps/api/internal/...`, `apps/api/cmd/server/...`;
one dashboard file under `apps/dashboard/src/...`. No new top-level directories.

---

## Phase 1: Setup

**Purpose**: Recorded JSON fixtures the adapter's tests will parse against — captured/
synthesized once, reused by every story's tests.

- [X] T001 [P] Create `apps/api/internal/jobsources/adapters/testdata/remoteok_api.json` — a
  representative RemoteOK `/api` response: a JSON array whose first element is the
  legal-notice object (no `id`/`position` field, per research.md R3) followed by several job
  objects with `id`, `position` (title), `company`, `location` (include at least one empty
  string), `tags` (array), `description` (HTML/text), `url`, `apply_url`, `date` (ISO 8601),
  and at least one entry with `salary_min`/`salary_max` present and one without
- [X] T002 [P] Create `apps/api/internal/jobsources/adapters/testdata/remoteok_empty.json` —
  a valid response containing only the legal-notice element and zero job objects (for the
  "zero results" edge case, FR-011/SC distinguishing "no matches" from "unparseable")

**Checkpoint**: Fixtures exist; nothing yet depends on live network access for tests.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The adapter must exist, compile, and be registered before any story's behavior
(search, management, enrichment) can be exercised end-to-end.

**⚠️ CRITICAL**: No user story task can be verified until this phase is complete.

- [X] T003 Create `apps/api/internal/jobsources/adapters/remoteok.go` with package
  boilerplate, the `RemoteOKAdapter` struct (`Scraping *scraping.Service` field, matching
  `IndeedAdapter`'s shape), `Key() string` returning `"remoteok"`, and
  `Kind() dto.SourceKind` returning `dto.SourceKindAPI`, per contracts/remoteok-adapter.md
- [X] T004 In `apps/api/internal/jobsources/adapters/remoteok.go`, add package-level
  constants `remoteokAPIURL` (`"https://remoteok.com/api"`) and `remoteokUserAgent` (a
  descriptive client identifier string per FR-017/research.md R2), and a helper
  `remoteokIsJobRecord(raw map[string]any) bool` that returns true only when both `id` and
  `position` keys are present (the legal-notice-skip rule from research.md R3)
- [X] T005 In `apps/api/internal/jobsources/adapters/remoteok.go`, implement
  `HealthCheck(ctx, config) (bool, error)` — fetch `remoteokAPIURL` via
  `d.Scraping.FetchHTML` with the `remoteokUserAgent` header, return `(false, nil)` on fetch
  error (never a non-nil error), `(true, nil)` when the response parses as JSON and contains
  at least the legal-notice element, per contracts/remoteok-adapter.md
- [X] T006 In `apps/api/cmd/server/compose.go`, construct
  `remoteokAdapter := adapters.RemoteOKAdapter{Scraping: p.Scraping}` near the existing
  `indeedAdapter` construction (~line 92) and add `remoteokAdapter` to the registry's
  adapter list (~lines 92-100); add a `RemoteOK adapters.RemoteOKAdapter` field to the
  sources-provider struct (~lines 79-109) alongside `Indeed`
- [X] T007 Verify `go build ./...` succeeds in `apps/api` with the new adapter registered but
  `Search`/`FetchDetail` still unimplemented stubs returning `not implemented` errors
  (compile-time checkpoint before story work begins)

**Checkpoint**: `RemoteOKAdapter` compiles, is registered in the runtime `Registry`, and
appears via `GET /api/sources` (registry-derived default row) — ready for story
implementation.

---

## Phase 3: User Story 1 - Discover RemoteOK listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: Enabling RemoteOK and running it against a saved tag/category subscription adds
new, deduplicated listings to the job feed, attributed to RemoteOK.

**Independent Test**: Enable RemoteOK, save a tag subscription URL, trigger a run, confirm
new `Job` rows appear with `sourceKey="remoteok"`, `remote=true`, full required fields, and
that re-running produces zero new duplicates.

### Tests for User Story 1

- [X] T008 [P] [US1] Write `TestRemoteOKKey`/`TestRemoteOKKind` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` asserting `Key()=="remoteok"` and
  `Kind()==dto.SourceKindAPI`
- [X] T009 [P] [US1] Write `TestParseRemoteOKJobs` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` loading
  `testdata/remoteok_api.json`, asserting the legal-notice element is skipped, every parsed
  `dto.NormalizedJob` has `SourceKey=="remoteok"`, `Remote==true`, non-empty `Title`/`URL`,
  `ExternalID` set from `id`, `SalaryRaw` populated only for the entry that has
  `salary_min`/`salary_max`, and `Location` nil (not empty-string) for the empty-location
  entry, per research.md R4
- [X] T010 [P] [US1] Write `TestParseRemoteOKJobs_Empty` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` loading
  `testdata/remoteok_empty.json`, asserting zero jobs and no error (not a failure) —
  validates the "zero results ≠ error" distinction (FR-011)
- [X] T011 [P] [US1] Write `TestRemoteOKSearch_NoSubscriptionURL` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` asserting `Search` with an empty
  `query.SubscriptionURL` returns a non-nil error (keyword search out of scope, FR-014)
- [X] T012 [US1] Write `TestRemoteOKSearch_TagResolution` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` using an `httptest.Server`
  serving `testdata/remoteok_api.json`, asserting `Search` called with
  `query.SubscriptionURL = "https://remoteok.com/remote-golang-jobs"` issues exactly one
  request and returns the parsed jobs (single-fetch behavior, no pagination loop, per
  research.md R7)

### Implementation for User Story 1

- [X] T013 [US1] In `apps/api/internal/jobsources/adapters/remoteok.go`, implement
  `remoteokTagFromURL(subURL string) string` per research.md R6: parse
  `remote-<tag>-jobs` out of the path of a `remoteok.com`/`remoteok.io` URL, returning `""`
  when the URL is the bare API root or the tag can't be parsed
- [X] T014 [US1] In `apps/api/internal/jobsources/adapters/remoteok.go`, implement
  `parseRemoteOKJobs(body []byte) ([]dto.NormalizedJob, error)` per research.md R3/R4:
  unmarshal the JSON array, skip any element failing `remoteokIsJobRecord`, map each
  remaining element's fields to `dto.NormalizedJob` (title from `position`, `Remote: true`
  always, `SalaryRaw` formatted from `salary_min`/`salary_max` when present, `URL` preferring
  `url` over `apply_url`, `Description` from `description`, `PostedAt` reformatted from
  `date` to RFC3339, `tags` and any unmapped fields stashed on `Raw`); return a non-nil error
  only when the body isn't valid JSON at all (distinguishing "unparseable" from "zero
  results", FR-011)
- [X] T015 [US1] In `apps/api/internal/jobsources/adapters/remoteok.go`, implement
  `Search(ctx, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)`:
  if `query.SubscriptionURL == ""` return the keyword-not-implemented error (T011);
  otherwise resolve the tag via T013, build the request URL (`remoteokAPIURL` or
  `remoteokAPIURL + "?tags=" + tag`), fetch once via `d.Scraping.FetchHTML` with the
  `remoteokUserAgent` header, and parse via T014, per contracts/remoteok-adapter.md
- [X] T016 [US1] Run `go test ./apps/api/internal/jobsources/adapters/... -run RemoteOK -v`
  and fix until T008–T012 pass

**Checkpoint**: User Story 1 is independently functional — `Search` against a real or
fixture subscription URL returns normalized, deduplicatable jobs; existing
`ingestion.Handler.persistIfNew`/dedupe/matching/score paths need no change to consume them
(dedupe key and downstream enqueue already operate on any `dto.NormalizedJob`, per
data-model.md).

---

## Phase 4: User Story 2 - Manage the RemoteOK source like any other source (Priority: P2)

**Goal**: Operators enable/disable RemoteOK, run a health test, trigger a manual run, and
manage its subscription URL from the Sources screen, exactly like DOU/Djinni/Indeed.

**Independent Test**: From the Sources screen (or its REST endpoints), toggle RemoteOK, run
`/test`, save/reject a subscription URL, and confirm run history updates — none of it
touching other sources.

### Tests for User Story 2

- [X] T017 [P] [US2] Write `TestRemoteOKHealthCheck` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` using an `httptest.Server`,
  asserting `(true, nil)` on a 200 JSON response and `(false, nil)` (not an error) on a
  connection failure
- [X] T018 [P] [US2] Write `TestValidateRemoteOKSubscriptionURL` in
  `apps/api/internal/subscriptions/service_test.go` asserting a `remoteok.com`/`remoteok.io`
  URL (both the tag-listing form and the bare `/api` form) is accepted and a non-RemoteOK
  URL (e.g. `https://example.com/x`) is rejected with an error, for `sourceKey=="remoteok"`
  (FR-015)

### Implementation for User Story 2

- [X] T019 [US2] In `apps/api/internal/subscriptions/service.go`, extend
  `validateSubscriptionURL` with a `remoteok` case: parse the URL and reject (return an
  error, no insert) unless its host is `remoteok.com`, `remoteok.io`, or a subdomain of
  either — implements data-model.md's Subscription validation rule
- [X] T020 [US2] In `apps/dashboard/src/features/sources/SourcesPage.tsx`, add
  `{ key: 'remoteok', label: 'RemoteOK', placeholder: 'https://remoteok.com/remote-golang-jobs' }`
  to the `SUBSCRIPTION_SOURCES` array (~line 205-206) so RemoteOK appears in the "New
  Subscription" source picker
- [X] T021 [US2] Run
  `go test ./apps/api/internal/jobsources/adapters/... ./apps/api/internal/subscriptions/... -run RemoteOK -v`
  and fix until T017–T018 pass

**Checkpoint**: User Stories 1 AND 2 both work independently — RemoteOK is fully manageable
via the existing `/api/sources`, `/api/sources/{key}/test`, and `/api/subscriptions`
endpoints, and visible/selectable in the dashboard.

---

## Phase 5: User Story 3 - Enrich RemoteOK listings with full posting detail (Priority: P3)

**Goal**: After ingestion, an enrichment pass confirms a listing is still live and fills in
any fields not already captured, marking rotated-out listings unavailable rather than
discarding their summary data.

**Independent Test**: Ingest one RemoteOK listing, run the enrich task for it, confirm its
stored `description`/`tags`/`postedAt` are populated; confirm a listing no longer present in
the current API payload is marked unavailable while its existing summary data is preserved.

### Tests for User Story 3

- [X] T022 [P] [US3] Write `TestRemoteOKFetchDetail` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` using an `httptest.Server`
  serving `testdata/remoteok_api.json`, asserting `FetchDetail` called with a `jobURL`
  matching one of the fixture's job `url` values returns `Available: true` with
  `Description`/`Tags`/`PostedAt` populated from that record, per research.md R5
- [X] T023 [P] [US3] Write `TestRemoteOKFetchDetail_RotatedOut` in
  `apps/api/internal/jobsources/adapters/remoteok_test.go` using an `httptest.Server`
  serving `testdata/remoteok_api.json`, asserting `FetchDetail` called with a `jobURL` that
  matches no record in the fixture returns `RemoteOKDetailPatch{Available: false}, nil` (not
  an error), per contracts/remoteok-adapter.md
- [X] T024 [US3] Write `TestEnrichRemoteOK` in
  `apps/api/internal/enrichment/handler_test.go` (extend existing file) asserting
  `ProcessTask` with `job.SourceKey=="remoteok"` calls the RemoteOK adapter's `FetchDetail`
  and persists via `UpdateJobDetail` when `Available: true`, and that
  `Available: false` marks the job unavailable while leaving its existing summary fields
  untouched rather than propagating an error to asynq

### Implementation for User Story 3

- [X] T025 [US3] In `apps/api/internal/jobsources/adapters/remoteok.go`, define
  `RemoteOKDetailPatch struct { Description string; Tags []string; SalaryRaw *string;
  PostedAt *string; Available bool; Raw map[string]any }` per contracts/remoteok-adapter.md
- [X] T026 [US3] In `apps/api/internal/jobsources/adapters/remoteok.go`, implement
  `FetchDetail(ctx, jobURL string, config map[string]any) (RemoteOKDetailPatch, error)`:
  re-fetch `remoteokAPIURL` via `d.Scraping.FetchHTML`, parse via T014, locate the record
  whose `URL` (or an ID parsed from `jobURL`) matches, return
  `RemoteOKDetailPatch{Available: false}, nil` when no match is found, otherwise
  `Available: true` with the matched record's fields, per research.md R5
- [X] T027 [US3] In `apps/api/internal/enrichment/handler.go`, add a
  `remoteok adapters.RemoteOKAdapter` field to `Handler` and a matching parameter to
  `NewHandler(...)` (after the existing `indeed` param)
- [X] T028 [US3] In `apps/api/internal/enrichment/handler.go`, add
  `case "remoteok": err = h.enrichRemoteOK(ctx, payload, uid, job); return err` to the
  `switch job.SourceKey` in `ProcessTask` (~line 108-109)
- [X] T029 [US3] In `apps/api/internal/enrichment/handler.go`, implement
  `enrichRemoteOK(ctx, payload, uid, job)` mirroring `enrichIndeed`: call
  `h.remoteok.FetchDetail`, on `Available: false` mark the job unavailable (preserve summary
  data) rather than erroring, on `Available: true` `UpdateJobDetail` and
  `enqueueMatch`/`enqueueSalaryInfer`
- [X] T030 [US3] In `apps/api/internal/ingestion/handler.go`, extend the enrich-eligibility
  check in `persistIfNew` (~line 214) from
  `if j.SourceKey == "djinni" || j.SourceKey == "dou" || j.SourceKey == "indeed"` to also
  include `|| j.SourceKey == "remoteok"` so newly-ingested RemoteOK jobs are auto-enqueued
  for detail enrichment
- [X] T031 [US3] In `apps/api/cmd/server/compose.go`, thread `sources.RemoteOK` into the
  `enrichment.NewHandler(...)` call (~line 303), matching the pattern of
  `sources.Djinni, sources.Dou, sources.Workua, sources.Indeed`
- [X] T032 [US3] Run
  `go test ./apps/api/internal/jobsources/adapters/... ./apps/api/internal/enrichment/... -run RemoteOK -v`
  and fix until T022–T024 pass

**Checkpoint**: All three user stories are independently functional. RemoteOK listings
ingest, are manageable, and get enriched with full detail — matching DOU/Djinni/Indeed
parity end-to-end.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation across all stories; no new functional surface.

- [X] T033 Run `go build ./...` and `go vet ./...` in `apps/api` to confirm no unused
  params/dead code left from the phased wiring (e.g. stub errors from T007)
- [X] T034 Run `make test-lint` (all three suites — constitution IV) to confirm no cross-app
  regression from the `SourcesPage.tsx` and Go changes
- [X] T035 Walk through `specs/003-remoteok-job-provider/quickstart.md` end-to-end against a
  running local stack (`make up`), recording any deviation from the documented `curl`
  responses
- [X] T036 [P] Update `apps/api/internal/seed/subscriptions.go` and
  `apps/api/internal/seed/testdata.go` with one RemoteOK seed subscription and one or two
  seed `Job` rows (`sourceKey: "remoteok"`), mirroring the existing indeed seed entries, so
  local dev/demo data includes RemoteOK (plan.md Assumptions: "seed/demo data... follow the
  same conventions as existing sources")

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — fixture files can be written immediately, in
  parallel (T001-T002 touch different files)
- **Foundational (Phase 2)**: T003-T006 have no fixture dependency and could start in
  parallel with Phase 1, but must all complete — BLOCKS all user stories (registry must be
  wired before any story is testable end-to-end)
- **User Story 1 (Phase 3)**: Depends on Foundational (Phase 2) only
- **User Story 2 (Phase 4)**: Depends on Foundational (Phase 2) only — does NOT depend on
  US1's `Search`/parsing implementation (T013-T015); `HealthCheck` (T005) and subscription
  validation (T019) are independent of job-parsing logic
- **User Story 3 (Phase 5)**: Depends on Foundational (Phase 2); T030/T031 touch the same
  ingestion/compose wiring conceptually exercised in Phase 2/US1 but are additive lines, not
  conflicting edits — can start once Phase 2 is done, though T024 (integration test) is most
  meaningful once US1's `Search`+persist path (Phase 3) is also in place, since
  `parseRemoteOKJobs` (T014) is reused by `FetchDetail` (T026)
- **Polish (Phase 6)**: Depends on all three stories being complete

### User Story Dependencies

- **US1 (P1)**: No dependency on US2/US3 — a subscription can be created via direct SQL/API
  call for its own test without US2's validation task, and detail stays list-level (no
  liveness-recheck) without US3
- **US2 (P2)**: Independent of US1's parsing logic; only needs the Foundational
  registration (Phase 2) and the adapter's `HealthCheck` (T005)
- **US3 (P3)**: Depends on `parseRemoteOKJobs` (T014, part of US1) since `FetchDetail`
  reuses it; its own tests (T022-T024) can run against a directly-constructed `Job` row
  without going through a full `Search` call, but T014 must exist first

### Within Each User Story

- Tests before their corresponding implementation task where both are listed (e.g.,
  T008-T012 before T013-T015)
- Tag resolution (T013) and parsing (T014) before `Search` (T015)
- `RemoteOKDetailPatch` (T025) before `FetchDetail` (T026, reuses T014) before the
  enrichment handler wiring (T027-T029)

### Parallel Opportunities

- T001-T002 (both fixture files) — fully parallel
- T008-T011 (independent test functions, same file but non-overlapping — still safe to
  author in parallel and merge) — parallel-eligible in spirit; if strictly enforcing
  "different files" for [P], treat as sequential edits to one file instead
- T017-T018 — different files (`remoteok_test.go` vs `subscriptions/service_test.go`), fully
  parallel
- T022-T023 — same file, sequential in practice despite conceptual independence
- T036 — independent of T033-T035, can run in parallel

---

## Parallel Example: Phase 1 (Setup)

```bash
Task: "Create testdata/remoteok_api.json fixture"
Task: "Create testdata/remoteok_empty.json fixture"
```

## Parallel Example: Phase 4 (User Story 2)

```bash
Task: "Write TestRemoteOKHealthCheck in apps/api/internal/jobsources/adapters/remoteok_test.go"
Task: "Write TestValidateRemoteOKSubscriptionURL in apps/api/internal/subscriptions/service_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (fixtures) + Phase 2 (adapter skeleton + registration)
2. Complete Phase 3 (US1: tag resolution + JSON parsing + `Search`)
3. **STOP and VALIDATE**: run `go test -run RemoteOK`, then quickstart.md steps 1, 3
   (subscription create), 4, 5 manually against a real or locally-stubbed RemoteOK response
4. This alone delivers the core value: RemoteOK listings flowing into the job feed,
   deduplicated

### Incremental Delivery

1. Setup + Foundational → adapter registered, compiles, appears in `/api/sources`
2. Add US1 → RemoteOK listings ingest and dedupe → demoable MVP
3. Add US2 → operators can manage/validate/health-check it from the Sources screen
4. Add US3 → listings get liveness-recheck + full-detail enrichment
5. Polish → seed data, quickstart walkthrough, lint/build across all three language suites

### Parallel Team Strategy

With multiple developers, once Phase 2 (Foundational) lands:

- Developer A: Phase 3 (US1 — tag resolution/parsing/Search)
- Developer B: Phase 4 (US2 — HealthCheck already in Foundational; validation + dashboard
  entry)
- Developer C: Phase 5 (US3 — FetchDetail + enrichment wiring, starting once T014 lands)

All three converge on the same `remoteok.go` file for different methods, so in practice this
is best run as a fast sequence (Foundational → US1 → US2 → US3) by one implementer, or
coordinated closely if split, to avoid merge conflicts within
`remoteok.go`/`remoteok_test.go`.

---

## Notes

- [P] tasks = different files, no dependencies — verify before parallelizing; several
  nominally-parallel test tasks above share one file (`remoteok_test.go`) and are noted as
  sequential-in-practice
- [Story] label maps task to specific user story for traceability
- `HealthCheck`/`Search`/subscription-validation patterns already have a direct precedent in
  `indeed.go`/`subscriptions/service.go` — implementation tasks should read those first
  rather than designing the flow from scratch; the JSON-parsing specifics (legal-notice
  skip, field mapping) are the genuinely new part, documented in research.md R3/R4
- No DB migration in this feature — do not add one
- Commit after each task or logical group (constitution: small, per-feature commits — see
  project convention)
- Stop at any checkpoint to validate story independently
