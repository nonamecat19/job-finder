# Tasks: Glassdoor Job Source

**Input**: Design documents from `/specs/004-glassdoor-job-provider/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Included as part of each story's implementation tasks (not a separate optional
phase) — this codebase's convention (Constitution IV) is that every adapter ships with
table-driven fixture tests in the same change, matching `indeed_test.go`/`remoteok_test.go`.

**Organization**: Tasks are grouped by user story (US1/US2/US3, priorities from spec.md) so
each can be implemented and validated independently, per the RemoteOK/Indeed precedent.

## Path Conventions

Single Go web-service project (`apps/api`) plus one dashboard file — paths are absolute
within the repo, per plan.md's Project Structure.

---

## Phase 1: Setup

**Purpose**: Nothing new to scaffold — this feature reuses the existing adapter package,
registry, and build. No setup tasks beyond confirming the target files exist.

- [X] T001 Confirm `apps/api/internal/jobsources/adapters/` builds cleanly on current
      `master` (`cd apps/api && go build ./...`) before starting, so any pre-existing
      failures aren't mistaken for this feature's regressions

---

## Phase 2: Foundational

**Purpose**: No blocking shared infrastructure is required — the `jobsources.Adapter`
interface, `scraping.Service`, and registry already exist and are unchanged by this feature
(plan.md Constitution Check: no new pipeline). Skipping to user stories.

**Checkpoint**: N/A — proceed directly to Phase 3.

---

## Phase 3: User Story 1 - Discover Glassdoor listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: Glassdoor listings flow into the job feed, deduplicated and source-attributed,
for an operator-saved subscription.

**Independent Test**: Enable Glassdoor with a saved subscription, trigger a run, confirm new
job records appear in `GET /api/jobs?source=glassdoor` with title, company, location, remote
flag, URL, and description; re-running adds zero new duplicates.

### Implementation for User Story 1

- [X] T002 [US1] Capture one live Glassdoor search-results page's HTML as
      `apps/api/internal/jobsources/adapters/testdata/glassdoor_list.html`, a second page as
      `glassdoor_list_page2.html`, a no-results page as `glassdoor_empty.html`, and (if
      reproducible) a bot-challenge/interstitial page as `glassdoor_blocked.html` — mirrors
      how `indeed_list.html`/`indeed_list_page2.html`/`indeed_empty.html` were captured for
      002; note actual card markup/selectors observed for T003
- [X] T003 [US1] Create `apps/api/internal/jobsources/adapters/glassdoor.go` implementing
      `jobsources.Adapter`: `Key() string` returns `"glassdoor"`, `Kind() dto.SourceKind`
      returns `dto.SourceKindScrape`, `HealthCheck(ctx, config)` fetches the source and
      reports reachability, and `Search(ctx, query dto.SearchQuery, config map[string]any)
      ([]dto.NormalizedJob, error)` requires `query.SubscriptionURL` (reject with
      `fmt.Errorf` otherwise, matching `IndeedAdapter.Search`'s stance per research.md R2),
      fetches via `scraping.Service.FetchHTML` with a descriptive User-Agent (FR-017), and
      parses job cards defensively with goquery multi-selector fallbacks (research.md R3)
- [X] T004 [US1] In `glassdoor.go`, implement pagination over the pasted search-results URL
      using Glassdoor's `p=<page>` query parameter, capped at a `glassdoorMaxSubscriptionPages`
      constant, stopping on zero new cards or a first-card repeat (loop guard), matching
      `indeedMaxSubscriptionPages`/`seenFirstHref` conventions (research.md R3)
- [X] T005 [US1] In `glassdoor.go`, implement bot-challenge/blocked-response detection
      (absence of job-card markup combined with challenge-page heuristics) that causes
      `Search` to return a distinguishable error (not an empty-but-successful result),
      satisfying FR-011/FR-018 — do not retry aggressively or attempt to bypass (research.md
      R3)
- [X] T006 [US1] In `glassdoor.go`, implement `glassdoorJobFromCard` (or equivalent) mapping
      a parsed card to `dto.NormalizedJob`: `SourceKey: "glassdoor"`, `ExternalID` when
      present, `Title`, `Company`, `Location` (nil when absent), `Remote` derived from
      listing text via a `glassdoorRemoteRe`-style regex (data-model.md — NOT always-true
      unlike RemoteOK), `SalaryRaw` (nil when absent, research.md R4), `URL`, `Description`
      (summary at this stage), `PostedAt` parsed from Glassdoor's date/relative-time text,
      `Raw` carrying the estimate-vs-employer-stated salary flag; drop cards with empty
      `Title` or `URL` (data-model.md Validation Rules)
- [X] T007 [US1] Add request pacing to `glassdoor.go`'s pagination loop — no request faster
      than every 500ms — matching `indeedRequestDelay` (FR-010)
- [X] T008 [P] [US1] Create `apps/api/internal/jobsources/adapters/glassdoor_test.go` with
      table-driven tests against the fixtures from T002: parses `glassdoor_list.html` into
      expected jobs, paginates into `glassdoor_list_page2.html`, returns zero jobs (not an
      error) for `glassdoor_empty.html`, and returns a distinguishable blocked-error for
      `glassdoor_blocked.html` — mirrors `indeed_test.go`'s structure
- [X] T009 [US1] Add `validateGlassdoorSubscriptionURL(rawURL string) error` in
      `apps/api/internal/subscriptions/service.go`: rejects non-`glassdoor.com`/
      `*.glassdoor.com` hosts and single-job-posting path shapes, with a human-readable
      `fmt.Errorf` reason (mirrors `validateIndeedSubscriptionURL`, research.md R6); add a
      `case "glassdoor":` to the existing `validateSubscriptionURL` switch (same file, ~line
      107-116)
- [X] T010 [P] [US1] Add a `TestValidateGlassdoorSubscriptionURL` table (valid glassdoor.com
      search URL, valid `www.glassdoor.com` URL, wrong host, single-job-posting path) to
      `apps/api/internal/subscriptions/service_test.go`, mirroring the existing Indeed/
      RemoteOK validation test tables in that file
- [X] T011 [US1] Wire `GlassdoorAdapter` into `apps/api/cmd/server/compose.go`:
      instantiate `glassdoorAdapter := adapters.GlassdoorAdapter{Scraping: p.Scraping}`
      alongside the other adapters (~line 93-94), add it to the `jobsources.NewRegistry(...)`
      call (~line 96-106), add a `Glassdoor adapters.GlassdoorAdapter` field to
      `sourcesHandles` (~line 77-84) and set it in the returned struct (~line 108-118) —
      matches the `remoteokAdapter`/`RemoteOK` pattern exactly
- [X] T012 [P] [US1] Add `{ key: 'glassdoor', label: 'Glassdoor', placeholder:
      'https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_...htm' }` to the source
      dropdown array in `apps/dashboard/src/features/sources/SourcesPage.tsx` (~line 203-206,
      alongside the existing DOU/Djinni/Indeed/RemoteOK entries)
- [X] T013 [US1] Run `cd apps/api && go build ./... && go test ./internal/jobsources/...
      ./internal/subscriptions/...` and confirm the new adapter and validation tests pass

**Checkpoint**: Glassdoor is a selectable, saveable, runnable source that adds deduplicated
listings to the feed — User Story 1 is independently testable via quickstart.md steps 1-5.

---

## Phase 4: User Story 2 - Manage the Glassdoor source like any other source (Priority: P2)

**Goal**: Operators manage Glassdoor from the Sources screen exactly like every other
source — enable/disable, health test, manual run, run history.

**Independent Test**: From the Sources screen, toggle Glassdoor off/on, run a health test,
trigger a manual run, and verify run history/counts update, per quickstart.md steps 3 and 7-8.

### Implementation for User Story 2

- [X] T014 [US2] Verify (and adjust if needed) that `HealthCheck` added in T003 reports
      `false` with no panic on the `glassdoor_blocked.html`/unreachable cases, and `true` on
      a normal response — add/extend a `TestGlassdoorAdapter_HealthCheck` case in
      `glassdoor_test.go` covering blocked vs. reachable vs. unreachable
- [X] T015 [US2] Confirm run-outcome recording (enable/disable, run trigger, run history with
      succeeded/failed/partial + counts) requires no Glassdoor-specific code — the existing
      `ingestion.Handler`/`jobsources.Service` are registry-driven and already handle any
      registered adapter uniformly; if a `switch job.SourceKey`-style special case is found
      in `apps/api/internal/ingestion/handler.go` during this check, add a `"glassdoor"` case
      there matching the others — **found one**: `internal/ingestion/handler.go` line 214 has
      an explicit `if j.SourceKey == "djinni" || ... "remoteok"` gate deciding which sources
      get enrichment enqueued at all; added `|| j.SourceKey == "glassdoor"` there (without it,
      T017-T021's enrichment code would exist but never run)

**Checkpoint**: Glassdoor has full management parity with Indeed/RemoteOK on the Sources
screen — User Story 2 is independently testable via quickstart.md steps 3 and 7-8.

---

## Phase 5: User Story 3 - Enrich Glassdoor listings with full posting detail (Priority: P3)

**Goal**: A Glassdoor listing's full description, salary estimate, and posting date are
completed after initial ingestion.

**Independent Test**: Ingest one Glassdoor listing, run enrichment, confirm stored
description/salary/posting-date are completed (or the listing is marked unavailable if no
longer live), per quickstart.md step 6.

### Implementation for User Story 3

- [X] T016 [P] [US3] Capture a live Glassdoor job-detail page as
      `apps/api/internal/jobsources/adapters/testdata/glassdoor_detail.html`
- [X] T017 [US3] In `glassdoor.go`, add a `GlassdoorDetailPatch` struct (Description,
      SalaryRaw, PostedAt, Available, Raw) and `FetchDetail(ctx context.Context, jobURL
      string, config map[string]any) (GlassdoorDetailPatch, error)`: fetches the detail page,
      parses full description/salary/posting date, and returns `Available: false` with a nil
      error (not a fetch failure) when the posting is no longer live — matches
      `IndeedAdapter.FetchDetail`/`RemoteOKAdapter.FetchDetail` shape (research.md R5, spec
      edge case "listing no longer available")
- [X] T018 [P] [US3] Add `TestGlassdoorAdapter_FetchDetail` to `glassdoor_test.go` using the
      T016 fixture: asserts full description/salary/posting-date are captured, and a
      not-found case returns `Available: false, err: nil`
- [X] T019 [US3] Add a `glassdoor adapters.GlassdoorAdapter` field to `enrichment.Handler` and
      its `NewHandler` constructor in `apps/api/internal/enrichment/handler.go` (~line 23-40,
      alongside `indeed`/`remoteok`), and add `enrichGlassdoor(ctx, payload, uid, job)`
      following `enrichIndeed`'s structure exactly (~line 237-268): apply `delayFor
      ("glassdoor")`, call `FetchDetail`, on `!patch.Available` leave existing job data
      untouched and log (mirrors `enrichRemoteOK`'s `Available` branch, ~line 270-281),
      otherwise call `h.q.UpdateJobDetail` and `h.enqueueMatch`/`h.enqueueSalaryInfer`
- [X] T020 [US3] Add `case "glassdoor": err = h.enrichGlassdoor(ctx, payload, uid, job);
      return err` to the `switch job.SourceKey` in `ProcessTask`
      (`apps/api/internal/enrichment/handler.go` ~line 99-118)
- [X] T021 [US3] Update the `enrichment.NewHandler(...)` call site in
      `apps/api/cmd/server/compose.go` (~line 307) to pass `sources.Glassdoor`
- [X] T022 [US3] Run `cd apps/api && go build ./... && go test ./internal/enrichment/...
      ./internal/jobsources/...` and confirm enrichment tests pass

**Checkpoint**: Glassdoor listings are enrichable end-to-end — User Story 3 is independently
testable via quickstart.md step 6. All three user stories now complete.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Seed data, full-suite validation, and final quickstart pass.

- [X] T023 [P] Add a `glassdoor` entry to the `subscriptions` seed list in
      `apps/api/internal/seed/subscriptions.go` (~line 17-46), matching the
      `indeed`/`remoteok` entries' shape (sourceKey, name, url, enabled: true)
- [X] T024 [P] Add one sample Glassdoor job to `apps/api/internal/seed/testdata.go`
      (~line 242-247, alongside the `indeed`/`remoteok` sample jobs), with a realistic
      title/company/salary/URL/description
- [x] T025 (partial) Ran the Go suite (`go build ./... && go vet ./... && go test ./...`,
      all pass) and the dashboard suite (`npx vitest run`, 160/160 pass, plus `tsc --noEmit`
      clean) for the touched apps. Did **not** run `test-python` (jobspy-sidecar) — this
      feature makes no jobspy-sidecar changes (research.md R1) so it's unaffected, and did
      not run the Docker-based `make test-lint` target itself since it requires a live
      Postgres/Redis compose stack not available in this session — re-run `make test-lint`
      before merge to get the canonical green build.
- [ ] T026 Execute `specs/004-glassdoor-job-provider/quickstart.md` end-to-end against a
      locally running stack (`make up`) and confirm all 8 steps produce their expected
      outcomes — **not run this session** (no live stack available); given research.md R3's
      confirmed 100%-block finding, expect step 3 (enable + run) and step 7 (health check)
      to show Glassdoor as unhealthy/blocked out of the box unless the BrowserContext
      escalation (deferred follow-up, research.md R3) is done first

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — T001 is a quick sanity check.
- **Foundational (Phase 2)**: Empty — nothing blocks User Story 1.
- **User Story 1 (Phase 3)**: Depends on Phase 1 only. This is the MVP.
- **User Story 2 (Phase 4)**: Depends on US1's `GlassdoorAdapter`/registry wiring (T003,
  T011) existing — mostly verification, minimal new code.
- **User Story 3 (Phase 5)**: Depends on US1's `glassdoor.go` file and registry wiring
  (T003, T011) existing, since `FetchDetail` is added to the same adapter type.
- **Polish (Phase 6)**: Depends on US1 (seed data references the adapter existing) and,
  for T026, ideally all three stories complete.

### User Story Dependencies

- **US1 (P1)**: No dependencies on US2/US3 — fully independent, deliverable alone as MVP.
- **US2 (P2)**: Builds on US1's adapter/registry wiring; independently testable once US1 is
  done, but adds no new adapter code of its own (mostly a verification pass).
- **US3 (P3)**: Builds on US1's adapter file; independently testable once US1 is done,
  without requiring US2.

### Parallel Opportunities

- T008 and T010 (new test files) can run in parallel with each other once T003/T009 exist.
- T012 (dashboard) is fully independent of all Go-side tasks and can run in parallel with
  T002-T011.
- T016 and T018 (US3 fixture + test) can be prepared in parallel with US2's Phase 4 tasks
  once Phase 3 is complete.
- T023 and T024 (seed data) are independent of each other and of Phase 4/5 verification
  tasks — can run in parallel once US1 is complete.

---

## Parallel Example: User Story 1

```bash
# After T003 (glassdoor.go) and T009 (validateGlassdoorSubscriptionURL) exist:
Task: "Create glassdoor_test.go table-driven tests (T008)"
Task: "Add TestValidateGlassdoorSubscriptionURL table (T010)"
Task: "Add Glassdoor entry to SourcesPage.tsx dropdown (T012)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (T001).
2. Complete Phase 3 (User Story 1, T002-T013).
3. **STOP and VALIDATE**: run quickstart.md steps 1-5 against a local stack.
4. Deploy/demo if ready — Glassdoor listings are already flowing into the feed at this point.

### Incremental Delivery

1. Phase 1 → Phase 3 (US1) → validate → MVP ships.
2. Phase 4 (US2) → validate management parity → ship.
3. Phase 5 (US3) → validate enrichment → ship.
4. Phase 6 (Polish: seed data, full quickstart, `make test-lint`) → final validation.

## Notes

- No DB migration in this feature — `job_sources` rows are upserted at startup by the
  registry, same as every prior source (data-model.md).
- No `apps/jobspy-sidecar` or `packages/shared` changes — research.md R1 explicitly rejects
  the sidecar path for this feature.
- T005's blocked-response detection is the one place implementation may need to deviate from
  plan: if live testing shows plain HTTP is reliably and immediately blocked (0% success),
  escalating `Search`/`FetchDetail` to `scraping.Service.BrowserContext` is a contained,
  same-file follow-up per research.md R3 — flag it in the PR description rather than
  silently reaching for it.
