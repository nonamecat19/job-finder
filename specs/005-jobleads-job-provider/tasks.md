---

description: "Task list for feature 005-jobleads-job-provider"
---

# Tasks: JobLeads Job Source

**Input**: Design documents from `/specs/005-jobleads-job-provider/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md,
contracts/jobleads-adapter.md, quickstart.md

**Tests**: Included — the codebase's existing convention (constitution IV,
`djinni_test.go`/`indeed_test.go`) is fixture-based `go test` coverage for every adapter,
including a login-flow test against a fake HTTP server (mirrors Djinni); written per story
alongside the code they cover, not as a strict red-green-first TDD gate.

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

**Purpose**: Recorded HTML fixtures the adapter's tests will parse against — captured/
synthesized once, reused by every story's tests.

- [X] T001 [P] Create `apps/api/internal/jobsources/adapters/testdata/jobleads_login.html` —
  a synthetic JobLeads login-page fixture (a form recognizable by
  `jobLeadsIsLoginPage`/`djinniIsLoginPage`-style detection, e.g. a login form container plus
  a CSRF-style hidden input) used both to detect "still logged out" and, if JobLeads uses a
  token-based login form, to extract it during the login POST
- [X] T002 [P] Create `apps/api/internal/jobsources/adapters/testdata/jobleads_list.html` —
  a representative authenticated saved-search results page: several listing cards with
  title, company, location (include at least one empty), a remote/hybrid/onsite indicator on
  at least one card, salary text present on at least one card and absent on another, listing
  URL, and posting-date text; per research.md R5
- [X] T003 [P] Create `apps/api/internal/jobsources/adapters/testdata/jobleads_detail.html` —
  a single listing's detail page with full description text and a resolved posting date, per
  research.md R6
- [X] T004 [P] Create `apps/api/internal/jobsources/adapters/testdata/jobleads_empty.html` —
  a valid, logged-in saved-search results page containing zero listing cards (for the "zero
  results" edge case, FR-011 distinguishing "no matches" from "unparseable")

**Checkpoint**: Fixtures exist; nothing yet depends on live network access for tests.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The adapter and its session manager must exist, compile, and be registered
before any story's behavior (search, management, enrichment) can be exercised end-to-end.
Because JobLeads is login-gated (unlike RemoteOK), the session/login plumbing is
foundational, not story-specific — every story's `Search`/`HealthCheck`/`FetchDetail` call
needs a working `Ensure`/`Refresh` path.

**⚠️ CRITICAL**: No user story task can be verified until this phase is complete.

- [X] T005 In `apps/api/internal/config/config.go`, add `JobLeadsEmail string
  `mapstructure:"JOBLEADS_EMAIL"`` and `JobLeadsPassword string
  `mapstructure:"JOBLEADS_PASSWORD"`` fields to the `Config` struct, mirroring
  `DjinniEmail`/`DjinniPassword` (~line 83-84), and add `"JOBLEADS_EMAIL"`,
  `"JOBLEADS_PASSWORD"` to the secret-list (~line 166) so they're redacted from any config
  dump
- [X] T006 Create `apps/api/internal/jobsources/adapters/jobleads_session.go` with
  `JobLeadsSessionProvider` interface (`Ensure`, `Refresh`), `JobLeadsConfigStore` interface
  (`Config`, `Update`), and `JobLeadsSession` struct (`Sources`, `Email`, `Password`, `Key`,
  `Base`, internal `mu sync.Mutex`), mirroring `djinni_session.go`'s
  `DjinniSessionProvider`/`DjinniConfigStore`/`DjinniSession` exactly, per
  contracts/jobleads-adapter.md
- [X] T007 In `apps/api/internal/jobsources/adapters/jobleads_session.go`, implement
  `(*JobLeadsSession).Ensure(ctx) (string, error)`: read `sessionCookie` from
  `Sources.Config(ctx, "jobleads")`; if present return it; if absent AND both `Email` and
  `Password` are set, call `Refresh`; if absent and either credential is empty, return
  `("", nil)` (do NOT error here — the adapter's `Search`/`HealthCheck` call sites turn an
  empty cookie into the "credentials not configured" error per research.md R1), mirroring
  `DjinniSession.Ensure`
- [X] T008 In `apps/api/internal/jobsources/adapters/jobleads_session.go`, implement
  `jobLeadsLogin(ctx, base, email, password string) (string, error)` and
  `(*JobLeadsSession).Refresh(ctx) (string, error)`: perform JobLeads's login flow (fetch the
  login page for any CSRF-style token via the `jobleads_login.html` shape, POST credentials
  with a cookie jar, extract the resulting session cookie), persist it via
  `Sources.Update(ctx, "jobleads", nil, map[string]any{"sessionCookie": cookie})`, serialized
  by `mu` so concurrent workers don't stampede login — mirrors `djinniLogin` +
  `DjinniSession.Refresh` exactly, per contracts/jobleads-adapter.md
- [X] T009 Create `apps/api/internal/jobsources/adapters/jobleads.go` with package
  boilerplate, the `JobLeadsAdapter` struct (`Scraping *scraping.Service`, `Session
  JobLeadsSessionProvider` fields, matching `DjinniAdapter`'s shape), `Key() string`
  returning `"jobleads"`, `Kind() dto.SourceKind` returning `dto.SourceKindScrape`, and a
  package-level `jobleadsMaxSubscriptionPages` constant, per contracts/jobleads-adapter.md
- [X] T010 In `apps/api/internal/jobsources/adapters/jobleads.go`, implement
  `jobLeadsIsLoginPage(doc *goquery.Document) bool` and an `authHeaders`/`fetchDoc` pair
  mirroring `DjinniAdapter.authHeaders`/`DjinniAdapter.fetchDoc`: attach the session cookie
  header, detect a login-page response, trigger exactly one `Session.Refresh` + retry on
  detection, and return a distinguishable "authentication required" error if still on the
  login page after retry — per research.md R4 and contracts/jobleads-adapter.md
- [X] T011 In `apps/api/cmd/server/compose.go`, construct a `JobLeadsSession` (wired with
  `cfg.JobLeadsEmail`/`cfg.JobLeadsPassword`) and `jobleadsAdapter :=
  adapters.JobLeadsAdapter{Scraping: p.Scraping, Session: p.JobLeadsSession}` near the
  existing `djinniAdapter` construction (~line 91), add `jobleadsAdapter` to the registry's
  adapter list (~line 101), add a `JobLeads adapters.JobLeadsAdapter` field to the
  sources-provider struct (~line 79-116) alongside `Djinni`, and wire
  `p.JobLeadsSession.Sources = sourcesSvc` (~line 112) mirroring `p.DjinniSession.Sources`
- [X] T012 Verify `go build ./...` succeeds in `apps/api` with the new adapter and session
  registered but `Search`/`FetchDetail` still unimplemented stubs returning `not implemented`
  errors (compile-time checkpoint before story work begins)

**Checkpoint**: `JobLeadsAdapter` + `JobLeadsSession` compile, are registered in the runtime
`Registry`, and appear via `GET /api/sources` (registry-derived default row) — ready for
story implementation.

---

## Phase 3: User Story 1 - Discover JobLeads listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: Enabling JobLeads and running it against a saved-search subscription (with
credentials configured) adds new, deduplicated listings to the job feed, attributed to
JobLeads.

**Independent Test**: Enable JobLeads, save a saved-search subscription URL, trigger a run,
confirm new `Job` rows appear with `sourceKey="jobleads"`, full required fields, and that
re-running produces zero new duplicates.

### Tests for User Story 1

- [X] T013 [P] [US1] Write `TestJobLeadsKey`/`TestJobLeadsKind` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` asserting `Key()=="jobleads"` and
  `Kind()==dto.SourceKindScrape`
- [X] T014 [P] [US1] Write `TestParseJobLeadsListings` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` loading
  `testdata/jobleads_list.html`, asserting every parsed `dto.NormalizedJob` has
  `SourceKey=="jobleads"`, non-empty `Title`/`Company`/`URL`, `ExternalID` derived from the
  listing URL/path, `SalaryRaw` populated only for the card that has salary text, and
  `Location` empty (not a placeholder) for the empty-location card, per research.md R5
- [X] T015 [P] [US1] Write `TestParseJobLeadsListings_Empty` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` loading
  `testdata/jobleads_empty.html`, asserting zero jobs and no error (not a failure) —
  validates the "zero results ≠ error" distinction (FR-011)
- [X] T016 [P] [US1] Write `TestJobLeadsSearch_NoSubscriptionURL` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` asserting `Search` with an empty
  `query.SubscriptionURL` returns a non-nil error (keyword search out of scope, FR-014)
- [X] T017 [P] [US1] Write `TestJobLeadsSearch_NoCredentials` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` asserting `Search` with a
  `JobLeadsSession` backed by empty `Email`/`Password` returns a non-nil, distinguishable
  "credentials not configured" error without issuing any HTTP request, per
  contracts/jobleads-adapter.md
- [X] T018 [US1] Write `TestJobLeadsSearch_AuthenticatedFetch` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` using an `httptest.Server` serving
  `testdata/jobleads_list.html` for authenticated requests (asserting the `Cookie` header
  carries the session cookie returned by a stub `JobLeadsSessionProvider`), asserting
  `Search` called with a saved-search `query.SubscriptionURL` returns the parsed jobs

### Implementation for User Story 1

- [X] T019 [US1] In `apps/api/internal/jobsources/adapters/jobleads.go`, implement
  `parseJobLeadsListings(doc *goquery.Document) ([]dto.NormalizedJob, error)` per research.md
  R5: select listing cards, map each to `dto.NormalizedJob` (title, company, location,
  best-effort `Remote` text match mirroring `djinniRemoteRe`, `SalaryRaw` when present, `URL`
  resolved to an absolute URL, list-level `Description` summary, `PostedAt` parsed from
  posting-date text to RFC3339, `ExternalID` from the listing URL/path); return a non-nil
  error only when the page structure is entirely unrecognizable (distinguishing
  "unparseable" from "zero results", FR-011)
- [X] T020 [US1] In `apps/api/internal/jobsources/adapters/jobleads.go`, implement
  `Search(ctx, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)`:
  if `query.SubscriptionURL == ""` return the keyword-not-implemented error (T016); if
  `d.Session` has no usable credentials (`Ensure` returns `""` with nil error and no
  configured `Email`/`Password`) return the "credentials not configured" error (T017);
  otherwise page through the saved-search URL up to `jobleadsMaxSubscriptionPages`, pacing
  requests at least 500ms apart (FR-010), fetching each page via the `fetchDoc` login-retry
  wrapper (T010) and parsing via T019, per contracts/jobleads-adapter.md
- [X] T021 [US1] Run `go test ./apps/api/internal/jobsources/adapters/... -run JobLeads -v`
  and fix until T013–T018 pass

**Checkpoint**: User Story 1 is independently functional — `Search` against a real or
fixture subscription URL (with credentials configured) returns normalized, deduplicatable
jobs; existing `ingestion.Handler.persistIfNew`/dedupe/matching/score paths need no change to
consume them (dedupe key and downstream enqueue already operate on any `dto.NormalizedJob`,
per data-model.md).

---

## Phase 4: User Story 2 - Manage the JobLeads source like any other source (Priority: P2)

**Goal**: Operators enable/disable JobLeads, run a health test, trigger a manual run, and
manage its subscription URL from the Sources screen, exactly like DOU/Djinni/Indeed/RemoteOK.

**Independent Test**: From the Sources screen (or its REST endpoints), toggle JobLeads, run
`/test` (with and without credentials configured), save/reject a subscription URL, and
confirm run history updates — none of it touching other sources.

### Tests for User Story 2

- [X] T022 [P] [US2] Write `TestJobLeadsHealthCheck` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` using an `httptest.Server`,
  asserting `(true, nil)` on a successful authenticated fetch, `(false, nil)` (not an error)
  on a connection failure or unauthorized response, per contracts/jobleads-adapter.md
- [X] T023 [P] [US2] Write `TestValidateJobLeadsSubscriptionURL` in
  `apps/api/internal/subscriptions/service_test.go` asserting a `jobleads.com` (and
  subdomain) URL is accepted and a non-JobLeads URL (e.g. `https://example.com/x`) is
  rejected with an error, for `sourceKey=="jobleads"` (FR-015)

### Implementation for User Story 2

- [X] T024 [US2] In `apps/api/internal/jobsources/adapters/jobleads.go`, implement
  `HealthCheck(ctx, config) (bool, error)`: attempt an authenticated fetch of the
  saved-search root/account page via the `fetchDoc` login-retry wrapper, return `(false,
  nil)` on any fetch/auth failure (never a non-nil error), `(true, nil)` on a recognizable
  authenticated response, per contracts/jobleads-adapter.md
- [X] T025 [US2] In `apps/api/internal/subscriptions/service.go`, extend
  `validateSubscriptionURL` with a `jobleads` case: parse the URL and reject (return an
  error, no insert) unless its host is `jobleads.com` or a subdomain of it — implements
  data-model.md's Subscription validation rule, mirroring the existing `remoteok` case
- [X] T026 [US2] In `apps/dashboard/src/features/sources/SourcesPage.tsx`, add `{ key:
  'jobleads', label: 'JobLeads', placeholder: 'https://www.jobleads.com/job-search?...' }` to
  the `SUBSCRIPTION_SOURCES` array (~line 204-206) so JobLeads appears in the "New
  Subscription" source picker
- [X] T027 [US2] Run
  `go test ./apps/api/internal/jobsources/adapters/... ./apps/api/internal/subscriptions/... -run JobLeads -v`
  and fix until T022–T023 pass

**Checkpoint**: User Stories 1 AND 2 both work independently — JobLeads is fully manageable
via the existing `/api/sources`, `/api/sources/{key}/test`, and `/api/subscriptions`
endpoints, and visible/selectable in the dashboard.

---

## Phase 5: User Story 3 - Enrich JobLeads listings with full posting detail (Priority: P3)

**Goal**: After ingestion, an enrichment pass fetches the full listing detail page and fills
in the complete description and posting date, marking removed listings unavailable rather
than discarding their summary data.

**Independent Test**: Ingest one JobLeads listing, run the enrich task for it, confirm its
stored `description`/`postedAt` are populated; confirm a listing whose detail page reports
removed/expired is marked unavailable while its existing summary data is preserved.

### Tests for User Story 3

- [X] T028 [P] [US3] Write `TestJobLeadsFetchDetail` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` using an `httptest.Server` serving
  `testdata/jobleads_detail.html` for an authenticated request, asserting `FetchDetail`
  called with a matching `jobURL` returns `Available: true` with `Description`/`PostedAt`
  populated, per research.md R6
- [X] T029 [P] [US3] Write `TestJobLeadsFetchDetail_Unavailable` in
  `apps/api/internal/jobsources/adapters/jobleads_test.go` using an `httptest.Server`
  returning a "listing removed" response (404 or an empty/removed-listing page) for the
  requested `jobURL`, asserting `FetchDetail` returns `JobLeadsDetailPatch{Available: false},
  nil` (not an error), per contracts/jobleads-adapter.md
- [X] T030 [US3] Write `TestEnrichJobLeads` in
  `apps/api/internal/enrichment/handler_test.go` (extend existing file) asserting
  `ProcessTask` with `job.SourceKey=="jobleads"` calls the JobLeads adapter's `FetchDetail`
  and persists via `UpdateJobDetail` when `Available: true`, and that `Available: false`
  marks the job unavailable while leaving its existing summary fields untouched rather than
  propagating an error to asynq

### Implementation for User Story 3

- [X] T031 [US3] In `apps/api/internal/jobsources/adapters/jobleads.go`, define
  `JobLeadsDetailPatch struct { Description string; SalaryRaw *string; PostedAt *string;
  Available bool; Raw map[string]any }` per contracts/jobleads-adapter.md
- [X] T032 [US3] In `apps/api/internal/jobsources/adapters/jobleads.go`, implement
  `FetchDetail(ctx, jobURL string, config map[string]any) (JobLeadsDetailPatch, error)`:
  fetch `jobURL` via the `fetchDoc` login-retry wrapper (T010), return
  `JobLeadsDetailPatch{Available: false}, nil` when the page reports the listing
  removed/expired, otherwise `Available: true` with `Description`/`PostedAt` parsed from the
  detail page, per research.md R6
- [X] T033 [US3] In `apps/api/internal/enrichment/handler.go`, add a `jobleads
  adapters.JobLeadsAdapter` field to `Handler` and a matching parameter to `NewHandler(...)`
  (after the existing `glassdoor` param)
- [X] T034 [US3] In `apps/api/internal/enrichment/handler.go`, add `case "jobleads": err =
  h.enrichJobLeads(ctx, payload, uid, job); return err` to the `switch job.SourceKey` in
  `ProcessTask`
- [X] T035 [US3] In `apps/api/internal/enrichment/handler.go`, implement
  `enrichJobLeads(ctx, payload, uid, job)` mirroring `enrichDjinni`: call
  `h.jobleads.FetchDetail`, on `Available: false` mark the job unavailable (preserve summary
  data) rather than erroring, on `Available: true` `UpdateJobDetail` and
  `enqueueMatch`/`enqueueSalaryInfer`
- [X] T036 [US3] In `apps/api/internal/ingestion/handler.go`, extend the enrich-eligibility
  check in `persistIfNew` (~line 214) to also include `|| j.SourceKey == "jobleads"` so
  newly-ingested JobLeads jobs are auto-enqueued for detail enrichment
- [X] T037 [US3] In `apps/api/cmd/server/compose.go`, thread `sources.JobLeads` into the
  `enrichment.NewHandler(...)` call (~line 311), matching the pattern of `sources.Djinni,
  sources.Dou, sources.Workua, sources.Indeed, sources.RemoteOK, sources.Glassdoor`
- [X] T038 [US3] Run
  `go test ./apps/api/internal/jobsources/adapters/... ./apps/api/internal/enrichment/... -run JobLeads -v`
  and fix until T028–T030 pass

**Checkpoint**: All three user stories are independently functional. JobLeads listings
ingest, are manageable, and get enriched with full detail — matching DOU/Djinni/Indeed/
RemoteOK/Glassdoor parity end-to-end.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation across all stories; no new functional surface.

- [X] T039 Run `go build ./...` and `go vet ./...` in `apps/api` to confirm no unused
  params/dead code left from the phased wiring (e.g. stub errors from T012)
- [X] T040 Run `make test-lint` (all three suites — constitution IV) to confirm no cross-app
  regression from the `SourcesPage.tsx` and Go changes
- [ ] T041 Walk through `specs/005-jobleads-job-provider/quickstart.md` end-to-end against a
  running local stack (`make up`) with real `JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD` set,
  recording any deviation from the documented `curl` responses, including the
  credentials-missing and session-expiry steps
- [X] T042 [P] Update `apps/api/internal/seed/subscriptions.go` and
  `apps/api/internal/seed/testdata.go` with one JobLeads seed subscription and one or two
  seed `Job` rows (`sourceKey: "jobleads"`), mirroring the existing remoteok seed entries, so
  local dev/demo data includes JobLeads (plan.md Assumptions: "seed/demo data... follow the
  same conventions as existing sources")

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — fixture files can be written immediately, in
  parallel (T001-T004 touch different files)
- **Foundational (Phase 2)**: T005-T011 have no fixture dependency and could start in
  parallel with Phase 1, but must all complete in order (T005 → T006-T008 → T009-T010 → T011
  → T012) — BLOCKS all user stories (session + registry must be wired before any story is
  testable end-to-end)
- **User Story 1 (Phase 3)**: Depends on Foundational (Phase 2) only
- **User Story 2 (Phase 4)**: Depends on Foundational (Phase 2) only — does NOT depend on
  US1's `Search`/parsing implementation (T019-T020); `HealthCheck` (T024) and subscription
  validation (T025) are independent of job-parsing logic
- **User Story 3 (Phase 5)**: Depends on Foundational (Phase 2); T036/T037 touch the same
  ingestion/compose wiring conceptually exercised in Phase 2/US1 but are additive lines, not
  conflicting edits — can start once Phase 2 is done, though T030 (integration test) is most
  meaningful once US1's `Search`+persist path (Phase 3) is also in place, since the login-page
  detection/retry helper (T010) is reused by `FetchDetail` (T032)
- **Polish (Phase 6)**: Depends on all three stories being complete

### User Story Dependencies

- **US1 (P1)**: No dependency on US2/US3 — a subscription can be created via direct SQL/API
  call for its own test without US2's validation task, and detail stays list-level (no
  full-description enrichment) without US3
- **US2 (P2)**: Independent of US1's parsing logic; only needs the Foundational registration
  (Phase 2) and the adapter's `HealthCheck` (T024, itself dependent on T010's fetchDoc
  wrapper from Foundational)
- **US3 (P3)**: Depends on the login-retry `fetchDoc` helper (T010, part of Foundational) and
  benefits from `parseJobLeadsListings` (T019, part of US1) existing first since both parse
  JobLeads HTML; its own tests (T028-T030) can run against a directly-constructed `Job` row
  without going through a full `Search` call

### Within Each User Story

- Tests before their corresponding implementation task where both are listed (e.g.,
  T013-T018 before T019-T020)
- Listing parsing (T019) before `Search` (T020)
- `JobLeadsDetailPatch` (T031) before `FetchDetail` (T032, reuses T010) before the
  enrichment handler wiring (T033-T035)

### Parallel Opportunities

- T001-T004 (all four fixture files) — fully parallel
- T013-T017 (independent test functions, same file but non-overlapping — still safe to
  author in parallel and merge) — parallel-eligible in spirit; if strictly enforcing
  "different files" for [P], treat as sequential edits to one file instead
- T022-T023 — different files (`jobleads_test.go` vs `subscriptions/service_test.go`), fully
  parallel
- T028-T029 — same file, sequential in practice despite conceptual independence
- T042 — independent of T039-T041, can run in parallel

---

## Parallel Example: Phase 1 (Setup)

```bash
Task: "Create testdata/jobleads_login.html fixture"
Task: "Create testdata/jobleads_list.html fixture"
Task: "Create testdata/jobleads_detail.html fixture"
Task: "Create testdata/jobleads_empty.html fixture"
```

## Parallel Example: Phase 4 (User Story 2)

```bash
Task: "Write TestJobLeadsHealthCheck in apps/api/internal/jobsources/adapters/jobleads_test.go"
Task: "Write TestValidateJobLeadsSubscriptionURL in apps/api/internal/subscriptions/service_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (fixtures) + Phase 2 (session + adapter skeleton + registration)
2. Complete Phase 3 (US1: authenticated fetch + HTML parsing + `Search`)
3. **STOP and VALIDATE**: run `go test -run JobLeads`, then quickstart.md steps 1, 4-6
   manually against a real JobLeads account or locally-stubbed responses
4. This alone delivers the core value: JobLeads listings flowing into the job feed,
   deduplicated

### Incremental Delivery

1. Setup + Foundational → adapter + session registered, compiles, appears in `/api/sources`
2. Add US1 → JobLeads listings ingest and dedupe → demoable MVP
3. Add US2 → operators can manage/validate/health-check it from the Sources screen
4. Add US3 → listings get full-description enrichment + unavailable-listing handling
5. Polish → seed data, quickstart walkthrough, lint/build across all three language suites

### Parallel Team Strategy

With multiple developers, once Phase 2 (Foundational) lands:

- Developer A: Phase 3 (US1 — listing parsing/Search)
- Developer B: Phase 4 (US2 — HealthCheck already partly in Foundational's fetchDoc; add
  validation + dashboard entry)
- Developer C: Phase 5 (US3 — FetchDetail + enrichment wiring, starting once T010/T019 land)

All three converge on the same `jobleads.go`/`jobleads_session.go` files for different
methods, so in practice this is best run as a fast sequence (Foundational → US1 → US2 → US3)
by one implementer, or coordinated closely if split, to avoid merge conflicts.

---

## Notes

- [P] tasks = different files, no dependencies — verify before parallelizing; several
  nominally-parallel test tasks above share one file (`jobleads_test.go`) and are noted as
  sequential-in-practice
- [Story] label maps task to specific user story for traceability
- `Session`/`fetchDoc`/login-retry patterns already have a direct precedent in
  `djinni.go`/`djinni_session.go` — implementation tasks should read those first rather than
  designing the flow from scratch; the HTML-parsing specifics (listing card selectors, field
  mapping) are the genuinely new part, documented in research.md R5/R6
- No DB migration in this feature — do not add one
- Commit after each task or logical group (constitution: small, per-feature commits — see
  [[feedback_commit_granularity]])
- Stop at any checkpoint to validate story independently
