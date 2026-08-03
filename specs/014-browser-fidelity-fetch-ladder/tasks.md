---

description: "Task list for 014-browser-fidelity-fetch-ladder"
---

# Tasks: Browser-Fidelity Retrieval and Escalation Ladder

**Input**: Design documents from `/specs/014-browser-fidelity-fetch-ladder/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The spec makes tests part of the deliverable — SC-002 ("verified by tests
that substitute challenge and refusal responses for real content") and SC-007 ("verified with
every source enabled and running concurrently") are testable-by-construction criteria, and
Constitution IV requires Docker-backed integration coverage for cross-service behavior.

**Organization**: Grouped by user story. Go tests are colocated as `*_test.go` next to the code
under test, per existing repo convention (`internal/httpapi/sources_test.go`,
`internal/ingestion/handler_test.go`).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US5, mapping to the spec's user stories

## Path Conventions

Go backend: `apps/api/internal/...`. Dashboard: `apps/dashboard/src/...`. Shared TS types are
**generated** from Go DTOs via tygo (`make tygo-generate` → `packages/shared/src/generated.ts`)
— never hand-edited (Constitution III).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Dependencies and package skeleton

- [X] T001 Add the TLS-fingerprinting client dependency (`github.com/bogdanfinn/tls-client`, per research.md decision 1) to `apps/api/go.mod` and run `go mod tidy`
- [X] T002 Create the new package skeleton `apps/api/internal/retrieval/` with a package doc comment in `apps/api/internal/retrieval/retrieval.go` describing it as the single shared retrieval interface (FR-020)
- [X] T003 [P] Add retrieval configuration keys to `apps/api/internal/config/config.go`: browser identity version, per-host daily budget default, cooling-off threshold and base duration, cheap-rung re-test interval (FR-014, FR-026, FR-030); `FLARESOLVERR_URL` already exists at line 113
- [X] T004 [P] Give the `flaresolverr` service in `docker-compose.yml` and `docker-compose.prod.yml` an explicit port mapping and healthcheck so the top rung is reachable and probeable under the `scraping-extras` profile

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Types, schema, and storage every user story builds on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T005 Define `PageOutcome` (Status: Read/Challenged/Refused/Unparseable/Deferred, Method, Reason, URL) and `RunVerdict` (success/partial/blocked) types in `apps/api/internal/retrieval/outcome.go` per data-model.md
- [X] T006 Define `RetrievalMethod` rung keys (`direct`, `browser`, `flaresolverr`) with their fixed cost ordering and an `Available()` probe in `apps/api/internal/retrieval/ladder.go`
- [X] T007 Write the goose migration `apps/api/internal/db/migrations/00025_host_retrieval_state.sql` creating `host_retrieval_state` with the columns in data-model.md (host unique, identityVersion, currentRung, rungLastVerifiedAt, cookies jsonb, consecutiveBlocks, coolingOffUntil, lastBlockAt, lastBlockReason, crawlDelaySeconds, budgetPeriodStart, budgetUsed, budgetLimit, updatedAt) — next unused goose version after 00024, never reuse a number
- [X] T008 Add the `verdict`, `blockedCount`, and `blockReason` columns to `SourceRun` in the same migration `apps/api/internal/db/migrations/00025_host_retrieval_state.sql`, defaulting existing rows so current dashboard reads of `ok`/`found` keep working
- [X] T009 Write sqlc queries for host retrieval state in `apps/api/internal/db/queries/hostretrievalstate.sql` (get by host, upsert, clear-cookies, clear-rung, increment-budget, record-block, record-success) and regenerate with sqlc
- [X] T010 Extend `apps/api/internal/db/queries/sourcerun.sql` to read and write the new verdict columns and regenerate with sqlc
- [X] T011 Implement the `HostRetrievalState` repository in `apps/api/internal/retrieval/state.go` over the generated sqlc queries, encrypting the `cookies` column with the existing `internal/crypto` helpers used for `JobSource.config` secrets (research.md decision 5)
- [X] T012 [P] Write repository tests in `apps/api/internal/retrieval/state_test.go` covering upsert-by-host, cookie encryption round-trip, and budget-window rollover, using the existing `internal/dbtest` Postgres harness
- [X] T013 Define the `retrieval.Service` interface and `FetchRequest`/`FetchResult` structs in `apps/api/internal/retrieval/service.go` exactly as specified in `contracts/retrieval-service.md`, including the doc comment that a block is a `PageOutcome` and never a Go `error`

**Checkpoint**: Types, schema, and per-host storage exist — user stories can begin

---

## Phase 3: User Story 1 - Blocked sources start returning listings (Priority: P1) 🎯 MVP

**Goal**: Outbound requests present as a current, real browser — consistent headers, header
order, TLS/HTTP2 fingerprint, persisted per-host cookies, jittered pacing — so currently-refused
sources answer.

**Independent Test**: Point a currently-refused source at its normal target, run it repeatedly
across several days, and confirm real listings come back rather than a refusal or challenge page.

### Tests for User Story 1

- [ ] T014 [P] [US1] Write `apps/api/internal/retrieval/identity_test.go` asserting the configured `BrowserIdentity` is internally consistent — UA browser/version matches the TLS profile's browser/version and the platform client hints (FR-002, acceptance 4) — and that construction fails loudly on a mismatch
- [ ] T015 [P] [US1] Write `apps/api/internal/retrieval/direct_test.go` against an `httptest.Server` asserting the emitted request carries the full browser header set **in the configured order** and no Go-default headers leak (FR-001)
- [ ] T016 [P] [US1] Write `apps/api/internal/retrieval/state_cookie_test.go` asserting cookies issued on one fetch are replayed on the next fetch to the same host, are never sent to a different host (FR-007), and are discarded when `identityVersion` differs (FR-006, acceptance 6)

### Implementation for User Story 1

- [X] T017 [US1] Implement `BrowserIdentity` in `apps/api/internal/retrieval/identity.go` — Version, UserAgent, Platform, ordered Headers, TLSProfileID — as a single process-wide configured value with startup validation (FR-002, FR-004)
- [X] T018 [US1] Implement the `direct` rung in `apps/api/internal/retrieval/direct.go` using the tls-client transport with the identity's fingerprint profile and ordered headers, wrapped by the existing `ratelimit.Transport` as its base so per-host pacing is preserved unchanged (FR-001, FR-003)
- [X] T019 [US1] Implement per-host cookie jar load/save in `apps/api/internal/retrieval/state.go`, keyed by host and tagged with `identityVersion`, discarding state whose identity version no longer matches (FR-005, FR-006)
- [X] T020 [US1] Keep visitor state for a credentialed source separate from anonymous state on the same host in `apps/api/internal/retrieval/state.go` (FR-007)
- [X] T021 [US1] Populate a plausible same-site navigation context (`Referer`, `Sec-Fetch-*`) from `FetchRequest.RefererPage` in `apps/api/internal/retrieval/direct.go` (FR-008)
- [X] T022 [US1] Add bounded random jitter to the inter-request interval in `apps/api/internal/ratelimit/transport.go` so intervals to a host are never a fixed value, keeping the existing `DefaultRPS` as the mean (FR-009, acceptance 5)
- [ ] T023 [P] [US1] Write `apps/api/internal/ratelimit/transport_test.go` additions asserting successive intervals to one host vary rather than repeating a fixed value (FR-009)
- [X] T024 [US1] Route `scraping.Service.FetchHTML` and `HTTPClient()` in `apps/api/internal/scraping/scraping.go` through the `retrieval` direct rung, deleting the hardcoded Chrome/126 `userAgent` constant at line 20 so no path disagrees about identity (FR-004)
- [X] T025 [US1] Replace the per-adapter `User-Agent` constants in `apps/api/internal/jobsources/adapters/djinni_session.go` and `jobleads_session.go` with the shared identity, and point `apps/api/internal/jobsources/httpjson.go`'s `defaultClient` at the same transport (FR-004)

**Checkpoint**: Requests are browser-faithful and remember hosts; previously-refused sources should start answering

---

## Phase 4: User Story 2 - Escalate only when actually challenged (Priority: P1)

**Goal**: Try the cheap rung first, escalate on a genuine challenge only, remember the rung that
worked per host, and never escalate for account-credentialed sources.

**Independent Test**: Run against a host that challenges the cheap request; confirm it escalates,
succeeds, records the rung, and starts there next run — while a host that answers cheaply never
escalates.

### Tests for User Story 2

- [ ] T026 [P] [US2] Write `apps/api/internal/retrieval/challenge_test.go` asserting challenge detection fires on challenge markup served under a 200, and does NOT fire on real content containing challenge-like wording (FR-012, edge case)
- [ ] T027 [P] [US2] Write `apps/api/internal/retrieval/ladder_test.go` covering contract behaviors 1–7 in `contracts/retrieval-service.md`: starts at recorded rung, retries the same URL at the next rung in one call, returns blocked as an `Outcome` with `err == nil`, records the succeeding rung, and resets `consecutiveBlocks`
- [ ] T028 [P] [US2] Write a test in `apps/api/internal/retrieval/ladder_test.go` asserting `FetchRequest.UsesUserAccount = true` never escalates past `direct` and reports the block for manual resolution (FR-018, acceptance 7)
- [ ] T029 [P] [US2] Write `apps/api/internal/retrieval/flaresolverr_test.go` asserting an unconfigured or unhealthy FlareSolverr yields a blocked/deferred `Outcome` with that stated reason and never a Go `error` that fails the run (FR-017, acceptance 6)
- [ ] T030 [P] [US2] Write `apps/api/internal/retrieval/browser_test.go` asserting the third-party chromedp allocator is a distinct instance from `scraping.Service.BrowserContext()`'s, and that a third-party render does not share or block the resume-PDF render path (FR-019, SC-012)

### Implementation for User Story 2

- [X] T031 [US2] Implement centralized challenge and refusal detection in `apps/api/internal/retrieval/challenge.go` — provider markup fingerprints plus structural signals (near-empty body, size far below the host's historical page size), judged on body and shape, never on status code alone (FR-012)
- [X] T032 [US2] Implement the `browser` rung in `apps/api/internal/retrieval/browser.go` as a **second, isolated** chromedp ExecAllocator with its own user-data-dir and lifecycle, separate from `scraping.Service.BrowserContext()` (FR-019, research.md decision 2)
- [X] T033 [US2] Add crash/hang/leak cleanup to `apps/api/internal/retrieval/browser.go`: a launch timeout, per-page context deadline, and teardown that reports the rung unavailable without holding up other sources (edge case)
- [X] T034 [US2] Implement the `flaresolverr` rung in `apps/api/internal/retrieval/flaresolverr.go` — request/response client against `cfg.FlaresolverrURL` (config.go:113, currently referenced by nothing) plus an availability health check (FR-010, FR-017)
- [X] T035 [US2] Implement ladder walking in `apps/api/internal/retrieval/ladder.go`: start at `HostRetrievalState.currentRung`, escalate only on a detected challenge or refusal, retry the same URL at the next rung within one `Fetch` call, stop at the top and return a blocked `Outcome` (FR-011, FR-016)
- [X] T036 [US2] Persist the succeeding rung as `currentRung` and reset `consecutiveBlocks` on any successful read in `apps/api/internal/retrieval/ladder.go` (FR-013)
- [X] T037 [US2] Implement periodic cheap-rung re-testing in `apps/api/internal/retrieval/ladder.go` driven by `rungLastVerifiedAt` and the configured interval, so no host is permanently pinned to an expensive rung (FR-014, acceptance 4)
- [X] T038 [US2] Suppress all escalation when `FetchRequest.UsesUserAccount` is set and report the block for manual resolution in `apps/api/internal/retrieval/ladder.go` (FR-018)
- [X] T039 [US2] Set `UsesUserAccount: true` on the account-credentialed adapters `apps/api/internal/jobsources/adapters/djinni_session.go` and `jobleads_session.go` (FR-018)
- [X] T040 [US2] Migrate every scraped adapter in `apps/api/internal/jobsources/adapters/` to fetch through `retrieval.Service`, and delete the ad hoc challenge string-matching currently in `wellfound.go:85-92`, `glassdoor.go:127`, and `jobgether.go:127` so no source implements its own retrieval or challenge handling (FR-020, SC-011)

**Checkpoint**: The ladder escalates only when challenged, remembers the rung, and hard-stops for account-credentialed sources

---

## Phase 5: User Story 3 - A blocked source never looks like an empty one (Priority: P1)

**Goal**: Every page outcome is classified and every run gets an honest verdict; repeated blocks
trigger a cooling-off period.

**Independent Test**: Feed a source challenge and refusal responses in place of real content and
confirm each run is reported as blocked with a reason — never as a successful empty run.

### Tests for User Story 3

- [ ] T041 [P] [US3] Write `apps/api/internal/ingestion/verdict_test.go` asserting a fully-blocked run yields `verdict = "blocked"` with a reason and is never reported as a successful zero-listings run (FR-021, SC-002)
- [ ] T042 [P] [US3] Add a case to `apps/api/internal/ingestion/verdict_test.go` asserting a run with some pages blocked and some read yields `verdict = "partial"` with the correct `blockedCount` while keeping the listings it read (FR-023)
- [ ] T043 [P] [US3] Add a case to `apps/api/internal/ingestion/verdict_test.go` asserting a source that returned listings on recent runs and now returns zero with no block detected is flagged as needing attention (FR-024, SC-003)
- [ ] T044 [P] [US3] Write `apps/api/internal/retrieval/coolingoff_test.go` asserting consecutive blocked runs trigger a cooling-off window, that the window lengthens on continued blocking, and that a cooling-off host is skipped with a `Deferred` outcome and no network request (FR-026, SC-010)

### Implementation for User Story 3

- [X] T045 [US3] Classify every retrieval into a `PageOutcome` in `apps/api/internal/retrieval/ladder.go`, including distinguishing `Unparseable` (read successfully but the parser found nothing) from `Challenged`/`Refused` (FR-022, edge case)
- [X] T046 [US3] Thread `PageOutcome` values from adapters up through the ingestion run in `apps/api/internal/ingestion/service.go` and `handler.go` so page-level outcomes survive to run aggregation
- [X] T047 [US3] Compute the `RunVerdict` (success / partial+blockedCount / blocked+reason) and write it to `SourceRun` in `apps/api/internal/ingestion/service.go`, so a blocked run can never be recorded as `ok` with zero listings (FR-021, FR-023)
- [ ] T048 [US3] Implement the zero-listings-after-recent-listings degradation flag in `apps/api/internal/ingestion/service.go`, derived from the last N `SourceRun.found` values compared against `verdict` (FR-024, data-model.md)
- [X] T049 [US3] Record `lastBlockAt` and `lastBlockReason` per host on every blocked outcome in `apps/api/internal/retrieval/state.go` (FR-025)
- [X] T050 [US3] Implement cooling-off in `apps/api/internal/retrieval/state.go`: increment `consecutiveBlocks`, set and lengthen `coolingOffUntil` past the configured threshold, and gate `Fetch` on it with a `Deferred` outcome before any network request (FR-026)
- [X] T051 [US3] Implement `OverrideCoolingOff` in `apps/api/internal/retrieval/service.go` returning the remaining duration for the caller to surface as risk, and leaving `coolingOffUntil` unchanged (FR-027, acceptance 5, edge case)
- [ ] T052 [US3] Honor a host's explicit wait instruction (`Retry-After`) as a floor on the next contact time in `apps/api/internal/retrieval/ladder.go` and `state.go` (FR-028)

**Checkpoint**: No run can report success while its pages were blocked; repeatedly-blocking hosts back off

---

## Phase 6: User Story 4 - The operator can see how each source is being retrieved (Priority: P2)

**Goal**: Per-host retrieval method, last block time/reason, cooling-off, and budget status are
visible on the Sources screen, with controls to clear rung preference and cookies.

**Independent Test**: Block a source, open the Sources screen, and confirm it shows the retrieval
method, last block reason and time, and any cooling-off period — without reading logs.

### Tests for User Story 4

- [ ] T053 [P] [US4] Write `apps/api/internal/httpapi/hosts_test.go` covering the four endpoints in `contracts/sources-api.md`: status read (and 404 for a never-contacted host), clear-rung-preference, clear-cookies, and override-cooling-off returning `remainingSeconds`
- [ ] T054 [P] [US4] Write dashboard tests in `apps/dashboard/src/features/sources/SourcesPage.test.tsx` asserting the host retrieval panel renders current rung, last block time/reason, and cooling-off state, and that the clear controls call their hooks

### Implementation for User Story 4

- [X] T055 [P] [US4] Add `HostRetrievalStatusDto`, `PageOutcomeDto`, and `RunVerdictDto` Go DTOs in `apps/api/internal/dto/` matching `contracts/sources-api.md`, then run `make tygo-generate` so `packages/shared/src/generated.ts` picks them up — never hand-edit the generated TS (Constitution III)
- [X] T056 [US4] Implement `GET /api/hosts/:host/retrieval-status` in `apps/api/internal/httpapi/hosts.go` returning `HostRetrievalStatusDto`, 404 when the host has no state row (FR-033)
- [X] T057 [US4] Implement `POST /api/hosts/:host/clear-rung-preference` and `POST /api/hosts/:host/clear-cookies` in `apps/api/internal/httpapi/hosts.go`, both idempotent, returning 204 (FR-015)
- [X] T058 [US4] Implement `POST /api/hosts/:host/override-cooling-off` in `apps/api/internal/httpapi/hosts.go` returning `{remainingSeconds}` without mutating the stored expiry (FR-027)
- [X] T059 [US4] Register the new host routes in `apps/api/internal/httpapi/router.go`
- [X] T060 [US4] Include the new `verdict` / `blockedCount` / `blockReason` fields in the existing recent-runs response in `apps/api/internal/httpapi/sources.go` (contracts/sources-api.md)
- [ ] T061 [P] [US4] Add `useHostRetrievalStatus`, `useClearRungPreference`, `useClearCookies`, and `useOverrideCoolingOff` hooks in `apps/dashboard/src/features/sources/hooks.ts`
- [ ] T062 [US4] Add a per-host retrieval panel to `apps/dashboard/src/features/sources/SourcesPage.tsx` showing current rung, last block time and reason, cooling-off state, and the clear-rung / clear-cookies controls (FR-033, User Story 4 acceptance 1–4)
- [ ] T063 [US4] Show run verdict (success / partial with blocked count / blocked with reason) instead of a bare `ok` flag in `RecentRunsPanel` in `apps/dashboard/src/features/sources/SourcesPage.tsx` (FR-021, SC-004)

**Checkpoint**: An operator can diagnose any source failure from the Sources screen alone

---

## Phase 7: User Story 5 - Volume stays low enough to be unremarkable (Priority: P2)

**Goal**: A per-host daily budget and published crawl delays are enforced across all concurrent
sources together, with over-budget requests deferred rather than failed.

**Independent Test**: Run every enabled source concurrently and confirm no host exceeds its daily
budget and per-host pacing holds regardless of how many sources target it.

### Tests for User Story 5

- [ ] T064 [P] [US5] Write `apps/api/internal/retrieval/budget_test.go` asserting a host's daily budget is shared across concurrently-running sources and that requests beyond it return `Deferred` outcomes, not failures (FR-030, FR-031, SC-007)
- [X] T065 [P] [US5] Add a case to `apps/api/internal/retrieval/budget_test.go` asserting a published crawl delay slower than the system's own pacing is honored (FR-029)

### Implementation for User Story 5

- [~] T066 [US5] Implement the per-host daily budget gate — **SUPERSEDED**: migration 00029 dropped budget columns per spec 017
- [~] T067 [US5] Enforce the budget process-wide across concurrent ingest tasks — **SUPERSEDED**: migration 00029 dropped budget columns per spec 017
- [X] T068 [US5] Fetch and honor each host's published `robots.txt` crawl delay in `apps/api/internal/retrieval/ladder.go`, caching it in `HostRetrievalState.crawlDelaySeconds` and applying it whenever it is slower than the system's own pacing (FR-029)
- [ ] T069 [US5] Surface budget exhaustion and reset time on the Sources screen: add `budgetUsed`/`budgetLimit`/`budgetResetsAt` display to the host panel in `apps/dashboard/src/features/sources/SourcesPage.tsx` (FR-034)

**Checkpoint**: Volume per host is bounded and observable regardless of how many sources are added

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T070 Add a guard test in `apps/api/internal/retrieval/` asserting no retrieval path is configurable to route through a third-party proxy, scraping service, or anonymizing relay (FR-032, SC-008)
- [ ] T071 [P] Run `make test-lint` across `apps/api` and `apps/dashboard` — required by Constitution IV for a change spanning both apps
- [X] T072 [P] Add an integration test under `apps/api/internal/db/` or `internal/retrieval/` exercising the full ladder against a Compose-started `flaresolverr` with real Postgres, per Constitution IV's no-mocks rule for cross-service behavior
- [ ] T073 [P] Verify the already-working adapters (arbeitnow, remotive, adzuna, jooble, remoteok) still return listings unchanged after the retrieval migration — any behavior change is a regression per spec Assumptions
- [ ] T074 [P] Document the retrieval ladder, browser identity upgrade procedure, and the `scraping-extras` profile requirement in the repo docs
- [ ] T075 Run every scenario in `specs/014-browser-fidelity-fetch-ladder/quickstart.md` end to end

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational. No dependency on other stories
- **US2 (Phase 4)**: Depends on Foundational. Uses US1's `direct` rung as its cheapest rung — implement US1 first for a coherent MVP, though the ladder is testable against a stub rung
- **US3 (Phase 5)**: Depends on Foundational. Consumes `PageOutcome` from US2's ladder; testable independently with a stubbed `retrieval.Service`
- **US4 (Phase 6)**: Depends on Foundational (state table) and reads what US2/US3 write. Independently testable with seeded state rows
- **US5 (Phase 7)**: Depends on Foundational (budget columns). Independent of US2–US4
- **Polish (Phase 8)**: Depends on all desired stories

### Within Each User Story

- Tests first; confirm they fail before implementing
- Types and state before ladder rungs; rungs before adapter migration
- Backend DTOs and `make tygo-generate` before any dashboard work

### Parallel Opportunities

- T003 and T004 in Setup
- T012 alongside T013 in Foundational
- All test tasks within a story (T014–T016; T026–T030; T041–T044; T053–T054; T064–T065)
- T055 and T061 can precede the UI task they feed
- US4 and US5 can be built in parallel by different developers once Foundational is done
- T071–T074 in Polish

---

## Parallel Example: User Story 2

```bash
# Launch all US2 tests together, before implementation:
Task: "Challenge detection tests in apps/api/internal/retrieval/challenge_test.go"
Task: "Ladder contract behaviors 1-7 in apps/api/internal/retrieval/ladder_test.go"
Task: "No-escalation-for-account-sources test in apps/api/internal/retrieval/ladder_test.go"
Task: "FlareSolverr-unavailable test in apps/api/internal/retrieval/flaresolverr_test.go"
Task: "Browser isolation test in apps/api/internal/retrieval/browser_test.go"

# Then the three rungs, which live in different files:
Task: "Implement browser rung in apps/api/internal/retrieval/browser.go"
Task: "Implement flaresolverr rung in apps/api/internal/retrieval/flaresolverr.go"
```

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1 Setup → Phase 2 Foundational → Phase 3 US1
2. **STOP and VALIDATE**: run a currently-refused source and confirm real listings return
3. US1 alone is shippable — browser fidelity plus persisted cookies may unblock several sources
   with no ladder at all, which is the spec's largest single hole

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US1 → validate → ship (MVP: sources answer)
3. US2 → validate → ship (hardest sources reachable, cost stays low)
4. US3 → validate → ship (no more silent zero-listing failures)
5. US4 → validate → ship (operator can diagnose)
6. US5 → validate → ship (volume bounded as the roster grows)

### Notes

- Commit per task or per logical group, small and per-feature
- Goose migration numbers are unique and sequential — 00025 is next; never reuse
- `packages/shared/src/generated.ts` is generated by tygo; regenerate, never hand-edit
- `make test-lint` is required before this change is done, since it touches two apps
