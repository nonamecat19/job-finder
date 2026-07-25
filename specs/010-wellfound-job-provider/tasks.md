# Tasks: Wellfound Job Source

**Input**: Design documents from `/specs/010-wellfound-job-provider/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, quickstart.md

**Tests**: Included — plan.md's Testing section and the Constitution Check (Principle IV) require `go test` coverage for the new adapter with recorded HTML fixtures, matching `glassdoor_test.go` convention.

**Organization**: Tasks are grouped by user story (US1/US2/US3, matching spec.md priorities P1/P2/P3) to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Paths are absolute-from-repo-root, matching plan.md's Project Structure section

## Path Conventions

Existing Go monolith (`apps/api`) + dashboard (`apps/dashboard`), following the Glassdoor adapter precedent exactly. No new project structure — this feature only adds/edits files inside the existing tree:

- `apps/api/internal/jobsources/adapters/` — new adapter + test + fixtures
- `apps/api/internal/subscriptions/service.go` — URL validation
- `apps/api/internal/ingestion/handler.go` — enrich-eligibility allowlist
- `apps/api/internal/enrichment/handler.go` — enrichment dispatch
- `apps/api/internal/seed/*.go` — seed data
- `apps/api/cmd/server/compose.go` — registration/wiring
- `apps/dashboard/src/features/sources/SourcesPage.tsx` — source picker entry

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Capture real Wellfound markup before any code is written, since research.md R4 flags field mapping as provisional pending real fixtures.

- [X] T001 Capture real Wellfound search-results HTML and save as fixtures in `apps/api/internal/jobsources/adapters/testdata/wellfound_list.html` (first page with multiple listings), `apps/api/internal/jobsources/adapters/testdata/wellfound_list_page2.html` (second page, for pagination tests), `apps/api/internal/jobsources/adapters/testdata/wellfound_empty.html` (zero-results state), `apps/api/internal/jobsources/adapters/testdata/wellfound_blocked.html` (bot-challenge/rate-limit page), and `apps/api/internal/jobsources/adapters/testdata/wellfound_detail.html` (single listing detail page with full description, qualifications, posting date, and salary/equity text)

**Checkpoint**: Fixtures exist and reflect real Wellfound markup — adapter implementation and tests in later phases can proceed against them.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: No shared infrastructure changes are required — this feature reuses `dto.NormalizedJob`, `jobsources.Adapter`, `scraping.Service`, and the `job_sources`/`Subscription`/`Job`/`SourceRun` tables unchanged (data-model.md: "No schema migration"). There is nothing that blocks all three user stories simultaneously beyond the adapter skeleton itself, which is scoped into US1 since US1 is the only story that requires it to exist at all.

**⚠️ CRITICAL**: This phase is intentionally empty per the plan's Constitution Check and data-model.md ("No new tables and no new migration"). Proceed directly to User Story 1.

**Checkpoint**: N/A — no foundational work; go straight to Phase 3.

---

## Phase 3: User Story 1 - Discover Wellfound listings alongside existing sources (Priority: P1) 🎯 MVP

**Goal**: A `WellfoundAdapter.Search` implementation that fetches an operator-configured Wellfound search-results URL, parses listing cards into `dto.NormalizedJob`, paces requests, bounds pagination, and is registered so ingestion picks up new listings, deduplicated and attributed to Wellfound like every other source.

**Independent Test** (from spec.md): Enable the Wellfound source, trigger a run, and confirm new job records appear in the feed tagged with Wellfound as their origin, each with title, company, location, remote flag, posting URL, and description. Re-running the same configuration adds zero new duplicate entries.

### Tests for User Story 1

> Write these tests FIRST using the fixtures from T001; ensure they FAIL before implementation.

- [X] T002 [P] [US1] Write `TestWellfoundAdapter_Search` table-driven tests in `apps/api/internal/jobsources/adapters/wellfound_test.go` covering: successful parse of `testdata/wellfound_list.html` into `dto.NormalizedJob` records (title, company, location, remote flag, salary/equity raw text, URL, description, external ID all populated per data-model.md's field mapping table), pagination across `testdata/wellfound_list_page2.html` up to the page cap, `testdata/wellfound_empty.html` returning zero results with a `succeeded` (not failed) outcome, `testdata/wellfound_blocked.html` returning a distinguishable "blocked" error, and a listing card missing `Title` or `URL` being skipped rather than emitted (data-model.md Validation Rules)
- [X] T003 [P] [US1] Write `TestWellfoundAdapter_HealthCheck` tests in `apps/api/internal/jobsources/adapters/wellfound_test.go` covering a reachable response, a blocked/challenge response, and an unreachable/network-error response, each producing a distinct human-readable reason

### Implementation for User Story 1

- [X] T004 [US1] Implement `adapters.WellfoundAdapter` struct (`Scraping *scraping.Service` field, matching `GlassdoorAdapter`'s shape) with `Key() string` returning `"wellfound"` and `Kind() dto.SourceKind` returning `dto.SourceKindScrape` in `apps/api/internal/jobsources/adapters/wellfound.go`
- [X] T005 [US1] Implement `WellfoundAdapter.Search(ctx, config, ...) ([]dto.NormalizedJob, error)` in `apps/api/internal/jobsources/adapters/wellfound.go`: fetch the operator-pasted search-results URL via `scraping.Service.FetchHTML`, parse listing cards with `goquery` per the field mapping in data-model.md/research.md R4 (title, company, location, remote-indicator regex akin to `glassdoorRemoteRe`, salary/equity raw text, canonical URL, summary description, stable `ExternalID` from the card URL/data attribute, best-effort `PostedAt`), skip cards missing `Title` or `URL`, and store any equity-vs-salary distinction in `Raw`
- [X] T006 [US1] Add a `wellfoundMaxSubscriptionPages` constant and a fixed inter-request delay (500ms, matching FR-010/research.md R7) to `apps/api/internal/jobsources/adapters/wellfound.go`, and implement bounded pagination in `Search` that stops at the page cap regardless of upstream "has more" signals
- [X] T007 [US1] Implement blocked-response detection in `apps/api/internal/jobsources/adapters/wellfound.go` (absence of recognizable job-card markup combined with challenge-page markers, per research.md R3) that returns a distinguishable "blocked" error from `Search`, separate from a legitimate zero-results parse
- [X] T008 [US1] Implement `WellfoundAdapter.HealthCheck(ctx) error` in `apps/api/internal/jobsources/adapters/wellfound.go`, reporting reachability/blocked/parse-failure as distinct human-readable reasons (FR-006, FR-011)
- [X] T009 [US1] Register `WellfoundAdapter` in `apps/api/cmd/server/compose.go`: add a `Wellfound adapters.WellfoundAdapter` field to the sources struct (mirroring line ~92's `Glassdoor adapters.GlassdoorAdapter`), construct `wellfoundAdapter := adapters.WellfoundAdapter{Scraping: p.Scraping}` (mirroring line ~105), add it to the adapter registration list (mirroring line ~116), and set `Wellfound: wellfoundAdapter` on the returned struct (mirroring line ~132)
- [X] T010 [US1] Add `"wellfound"` to the ingestion enrich-eligible source-key allowlist in `apps/api/internal/ingestion/handler.go` line ~220 (`persistIfNew`'s `j.SourceKey == "djinni" || ... || j.SourceKey == "glassdoor" || j.SourceKey == "jobleads"` condition)
- [X] T011 [P] [US1] Add a seeded Wellfound sample job in `apps/api/internal/seed/testdata.go` (matching the `SourceKey: "glassdoor"` entries around lines 196/252 — new job with `SourceKey: "wellfound"`, title, company, salary/equity raw text, URL)
- [X] T012 [P] [US1] Add a seeded Wellfound source-run record in `apps/api/internal/seed/sourceruns.go` (matching line 31's `{sourceKey: "glassdoor", searchID: "seed:glassdoor:ml-engineer"}` entry)

**Checkpoint**: At this point, enabling Wellfound and triggering a run produces deduplicated, feed-visible listings attributed to Wellfound — User Story 1 is independently functional and testable per its Independent Test.

---

## Phase 4: User Story 2 - Manage the Wellfound source like any other source (Priority: P2)

**Goal**: An operator can save/reject a Wellfound subscription URL, enable/disable the source, run a health check, trigger a manual run, and see Wellfound listed with state/health/run-summary on the Sources screen — all through the existing subscription/source management flow, no Wellfound-specific UI.

**Independent Test** (from spec.md): From the Sources screen, save a Wellfound subscription, toggle Wellfound off and on, run a health test, trigger a manual run, and verify run history and counts update — all without touching other sources' behavior.

### Tests for User Story 2

- [X] T013 [P] [US2] Write `TestValidateWellfoundSubscriptionURL` table-driven tests in `apps/api/internal/subscriptions/service_test.go` (or the existing subscriptions test file) covering: accepted `wellfound.com` search-results URL, accepted `.wellfound.com` subdomain, accepted legacy `angel.co` host, rejected non-Wellfound host (e.g. an Indeed URL) with a human-readable reason, and rejected single-job-posting URL shape with a human-readable reason (FR-015, SC-008, research.md R6)

### Implementation for User Story 2

- [X] T014 [US2] Add `case "wellfound": return validateWellfoundSubscriptionURL(rawURL)` to the `validateSubscriptionURL` switch in `apps/api/internal/subscriptions/service.go` around line 113 (mirroring the existing `case "glassdoor":` at line 113-114)
- [X] T015 [US2] Implement `validateWellfoundSubscriptionURL(rawURL string) error` in `apps/api/internal/subscriptions/service.go` (mirroring `validateGlassdoorSubscriptionURL` at lines 150-160): parse the URL, reject if the host is not `wellfound.com`/`*.wellfound.com`/`angel.co`, reject if the path shape looks like a single job posting rather than a search-results page, each with a human-readable `fmt.Errorf` reason (depends on T014)
- [X] T016 [P] [US2] Add a seeded Wellfound subscription in `apps/api/internal/seed/subscriptions.go` (mirroring lines 49-51's `sourceKey: "glassdoor"` / `name: "Glassdoor Go Remote"` / `url: "https://www.glassdoor.com/..."` entry) with `sourceKey: "wellfound"`, a descriptive name, and a Wellfound search-results URL
- [X] T017 [P] [US2] Add `{ key: 'wellfound', label: 'Wellfound', placeholder: 'https://wellfound.com/role/r/golang-engineer' }` to the subscription-form source picker array in `apps/dashboard/src/features/sources/SourcesPage.tsx` around line 220 (mirroring the existing `glassdoor` entry)

**Checkpoint**: At this point, Wellfound is fully manageable from the Sources screen (save/reject subscription, enable/disable, health check, manual run, run history) — User Story 2 is independently functional and testable per its Independent Test, without depending on US1's adapter internals beyond the `Key()`/`HealthCheck()` surface already delivered in Phase 3.

---

## Phase 5: User Story 3 - Enrich Wellfound listings with full posting detail (Priority: P3)

**Goal**: `WellfoundAdapter.FetchDetail` retrieves the full description, qualifications, resolved posting date, and availability for an already-ingested Wellfound listing, wired into the enrichment pipeline so matching/scoring/generation see the complete posting.

**Independent Test** (from spec.md): Ingest one Wellfound listing, run enrichment for it, and confirm the stored description, qualifications, and compensation range are captured in full, with posting date resolved. A removed/session-gated listing is marked unavailable while its summary data is preserved.

### Tests for User Story 3

- [X] T018 [P] [US3] Write `TestWellfoundAdapter_FetchDetail` table-driven tests in `apps/api/internal/jobsources/adapters/wellfound_test.go` covering: `testdata/wellfound_detail.html` parsed into full description (folding in qualifications), resolved `PostedAt`, and `Available: true`; a "listing no longer available" / 404 / removed-listing response producing `Available: false` while the patch does not clear already-captured summary fields (FR-009 second acceptance scenario)

### Implementation for User Story 3

- [X] T019 [US3] Implement `WellfoundDetailPatch` struct and `WellfoundAdapter.FetchDetail(ctx, jobURL, config) (WellfoundDetailPatch, error)` in `apps/api/internal/jobsources/adapters/wellfound.go` (mirroring `GlassdoorDetailPatch`/`IndeedDetailPatch` shape per research.md R5): fetch the detail page, extract full description + qualifications text, resolved posting date, and detect "no longer available"/session-gated responses to set `Available: false` without discarding summary data
- [X] T020 [US3] Add a `wellfound adapters.WellfoundAdapter` field and constructor parameter to `enrichment.Handler`/`NewHandler` in `apps/api/internal/enrichment/handler.go` (mirroring the `glassdoor adapters.GlassdoorAdapter` field at line ~31 and the `NewHandler` signature at line ~38), add a `case "wellfound": err = h.enrichWellfound(ctx, payload, uid, job)` dispatch case (mirroring line ~117-118), and implement `enrichWellfound` (mirroring `enrichGlassdoor` at lines ~346-378): apply the source-specific enrichment delay, call `h.wellfound.FetchDetail`, log and preserve existing data on an "unavailable" result, otherwise persist the full description/posting date update
- [X] T021 [US3] Update the `enrichment.NewHandler` call site in `apps/api/cmd/server/compose.go` line ~330 to pass `sources.Wellfound` as the new constructor argument (mirroring how `sources.Glassdoor` is threaded through today)

**Checkpoint**: All three user stories are independently functional — Wellfound listings are discovered (US1), manageable (US2), and enriched with full detail (US3).

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Validation and regression checks that span all three stories.

- [X] T022 [P] Run `cd apps/api && go test ./...` and confirm the full suite passes, including the new `wellfound_test.go` cases, with no live network calls (Constitution Principle IV)
- [X] T023 [P] Run `cd apps/dashboard && <existing test command>` and confirm the `SourcesPage.tsx` change doesn't break existing dashboard tests
- [ ] T024 Execute `specs/010-wellfound-job-provider/quickstart.md` end-to-end against a running local stack (`make up`): confirm source registration (FR-001/FR-016), subscription save/reject (FR-014/FR-015/SC-008), enable/run/health-check (FR-005/FR-006), feed visibility and dedup (FR-002/FR-003/FR-004/SC-003/SC-004), source filtering in the feed (US1 scenario 3), enrichment (FR-009/SC-006), and pacing/page-cap behavior (FR-010)
- [ ] T025 Confirm SC-007 (adding Wellfound does not increase the median end-to-end ingestion cycle time for existing sources by more than 10%) by comparing an ingestion cycle's timing before/after this feature, or by inspection that Wellfound's pacing/page-cap runs on its own schedule slot rather than serializing with other sources

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — capture fixtures first since all adapter tests and implementation reference them
- **Foundational (Phase 2)**: Empty — no schema/shared-infrastructure changes needed (data-model.md: no migration)
- **User Story 1 (Phase 3)**: Depends on Phase 1 fixtures. No dependency on US2/US3
- **User Story 2 (Phase 4)**: Depends on Phase 1 only for validation tests' realism (does not strictly need US1's adapter code — `validateWellfoundSubscriptionURL` is pure URL logic). Independently testable even before US1's `Search` is complete, though a full "save + run" flow needs US1's adapter registered
- **User Story 3 (Phase 5)**: Depends on US1's `WellfoundAdapter` struct and its registration in `compose.go` (T004, T009) existing before `FetchDetail` can be added to the same struct and wired into enrichment
- **Polish (Phase 6)**: Depends on all three user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: No dependency on other stories — the MVP slice
- **User Story 2 (P2)**: Independently testable; benefits from US1 existing so a saved subscription can actually be run end-to-end, but its own tests (T013) and validation code (T014-T015) have no code dependency on US1
- **User Story 3 (P3)**: Depends on US1's adapter struct (`WellfoundAdapter`) existing in `apps/api/internal/jobsources/adapters/wellfound.go` — `FetchDetail` is added to that same struct

### Within Each User Story

- Tests before implementation (T002-T003 before T004-T012; T013 before T014-T017; T018 before T019-T021)
- Adapter core (`Key`/`Kind`/`Search`) before registration (`compose.go`) before allowlist wiring
- `FetchDetail` (US3) after the adapter struct exists (US1)

### Parallel Opportunities

- T002 and T003 (different test functions, same file — can be drafted in parallel then merged, or treated as sequential edits to the same file if strict [P] file-isolation is preferred)
- T011 and T012 (different seed files) can run in parallel
- T016 and T017 (different files: Go seed vs. TSX) can run in parallel, and both in parallel with T013-T015
- T022 and T023 (different test suites: Go vs. dashboard) can run in parallel

---

## Parallel Example: User Story 1

```bash
# Tests first (can be drafted in parallel, both target wellfound_test.go):
Task: "Write TestWellfoundAdapter_Search in apps/api/internal/jobsources/adapters/wellfound_test.go"
Task: "Write TestWellfoundAdapter_HealthCheck in apps/api/internal/jobsources/adapters/wellfound_test.go"

# Seed data, once the adapter core exists:
Task: "Add seeded Wellfound sample job in apps/api/internal/seed/testdata.go"
Task: "Add seeded Wellfound source-run record in apps/api/internal/seed/sourceruns.go"
```

## Parallel Example: User Story 2

```bash
Task: "Write TestValidateWellfoundSubscriptionURL in apps/api/internal/subscriptions/service_test.go"
Task: "Add seeded Wellfound subscription in apps/api/internal/seed/subscriptions.go"
Task: "Add wellfound entry to apps/dashboard/src/features/sources/SourcesPage.tsx source picker"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (capture real fixtures — T001)
2. Skip Phase 2: Foundational is empty for this feature
3. Complete Phase 3: User Story 1 (T002-T012)
4. **STOP and VALIDATE**: Enable Wellfound, trigger a run, confirm listings appear in the feed and re-running produces zero duplicates (per US1's Independent Test)
5. Deploy/demo if ready — Wellfound listings now flow into the feed even without dedicated management UI polish or enrichment

### Incremental Delivery

1. Setup (fixtures) → Phase 3 (US1) → Test independently → Deploy/Demo (MVP: listings flow into the feed)
2. Add Phase 4 (US2) → Test independently → Deploy/Demo (operators can manage Wellfound like any other source)
3. Add Phase 5 (US3) → Test independently → Deploy/Demo (full description/qualifications/posting date after enrichment)
4. Phase 6 (Polish) → run quickstart.md end-to-end, confirm SC-007 cycle-time budget

### Parallel Team Strategy

With multiple developers, after Phase 1 fixtures exist:

1. Developer A: User Story 1 (adapter core, `compose.go` registration, ingestion allowlist)
2. Developer B: User Story 2 (subscription URL validation, seed subscription, dashboard picker entry) — can start immediately since validation logic has no code dependency on US1
3. Developer C: User Story 3 (`FetchDetail` + enrichment wiring) — starts once Developer A's `WellfoundAdapter` struct skeleton (T004) lands, since `FetchDetail` is added to that same struct

---

## Notes

- [P] tasks touch different files (or are logically independent within a shared test file) and have no unmet dependencies
- [Story] label maps each task to US1/US2/US3 for traceability back to spec.md's prioritized user stories
- No schema migration in this feature (data-model.md) — Phase 2 (Foundational) is deliberately empty
- Field mapping in T005 is provisional per research.md R4 until T001's real fixtures are captured; adjust selectors as needed once real markup is available
- Constitution Principle IV requires per-language test discipline: all new Go tests run against recorded fixtures, no live Wellfound network calls in `go test`
- Verify tests fail before implementing (T002/T003 before T004-T012; T013 before T014-T017; T018 before T019-T021)
- Commit after each task or logical group, per repository convention (small, per-feature commits)
