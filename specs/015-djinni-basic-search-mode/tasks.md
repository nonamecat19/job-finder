# Tasks: Djinni Basic-Search Mode

**Input**: Design documents from `/specs/015-djinni-basic-search-mode/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/djinni-url-shapes.md, quickstart.md

**Tests**: Included — constitution IV mandates per-language test discipline, and the constitution-IV quality gate (`make test-lint`) is itself a quickstart step. Constitution III (no hand-maintained duplicate cross-app types) is preserved by keeping the URL as the single contract surface.

**Organization**: Tasks are grouped by user story. Each user story maps to a phase from the implementation plan.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Backend (Go)**: `apps/api/internal/...`
- **Frontend (TS)**: `apps/dashboard/src/...`
- Path roots are repository-relative; the implementation plan in `plan.md` shows the full per-app layout.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No project to initialize — the repo, the `djinni` source, the `subscriptions` flow, and the `SourcesPage.tsx` already exist. Setup is just scaffolding the two new files so subsequent tasks land in real, named files rather than in midair.

- [X] T001 [P] Create empty `apps/api/internal/jobsources/adapters/djinni_searchmode.go` with a file-level doc comment referencing spec FR-002, research.md R2, and `contracts/djinni-url-shapes.md`. Add only the `package adapters` declaration and the doc-comment block — no types or functions yet.
- [X] T002 [P] Create empty `apps/dashboard/src/features/sources/djinniSearchSummary.ts` with a file-level doc comment referencing spec FR-008/FR-009, research.md R3/R4, and `contracts/djinni-url-shapes.md`. Add only the doc comment — no functions yet.

**Checkpoint**: Two new files exist with documented intent and correct package/path. No behavior change yet; `go test ./...` and `pnpm --filter @job-finder/dashboard typecheck` still pass.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The URL-shape contract + the shared `exp_level`-range rule — pure logic with no HTTP dependency. Both apps consume this; both modes (Dashboard, BasicSearch) and both user stories (run, display) depend on it. This phase MUST be complete before any user-story task.

**⚠️ CRITICAL**: No user-story work can begin until T003–T006 (Go) and T007–T008 (TS) pass their unit tests.

- [X] T003 [US1] Define `DjinniSearchMode` enum, `DjinniDetect(rawURL string) DjinniSearchMode`, and `BasicSearchFilters` struct in `apps/api/internal/jobsources/adapters/djinni_searchmode.go` per `contracts/djinni-url-shapes.md` §1–§3. Host check accepts `djinni.co` and `www.djinni.co`. Dashboard shape matches `/my/dashboard/subs/<id>/`. Basic-search shape requires path `/jobs` or `/jobs/` AND `search_type=basic-search` query param. Anything else → `DjinniModeUnknown`. No HTTP, no `*goquery.Document`, no `*scraping.Service` — pure `net/url` only.
- [X] T004 [US2] Implement `summarizeExpLevels(values []string) string` in `apps/api/internal/jobsources/adapters/djinni_searchmode.go` per `contracts/djinni-url-shapes.md` §4. Deduplicate via a set, sort ascending, collapse consecutive set (adjacent gaps == 1) to `"min–max years"` (en-dash `–`), single value to `"n years"`, non-consecutive to `"a, b[, c] years"`. Non-parseable `Ny` tokens fall back to the non-consecutive list rendering (never mis-collapse). Used by Go-side logging and validator reason strings.
- [X] T005 [US1] Add `ParseBasicSearch(rawURL string) (BasicSearchFilters, bool)` in `apps/api/internal/jobsources/adapters/djinni_searchmode.go` returning `(filters, true)` when `DjinniDetect == BasicSearch`, else zero-value/`false`. Populates `PrimaryKeyword`, `Salary`, `ExpLevels []string`, `Employment` from the corresponding query params; preserves full param names/values as received (no normalization, no currency conversion).
- [X] T006 [US1] Add table-driven tests in `apps/api/internal/jobsources/adapters/djinni_searchmode_test.go` covering `DjinniDetect` for: dashboard URL, dashboard URL with trailing path/no trailing slash, basic-search URL with all filters, basic-search URL with only `search_type`+`primary_keyword`, basic-search URL missing `search_type` (must NOT be BasicSearch → dashboard-shape mismatch or Unknown), single job-posting path (`/jobs/<id>` with `search_type` absent → Unknown), non-`djinni.co` host (Unknown), `www.djinni.co` host. Tests for `ParseBasicSearch` cover duplicate `exp_level` values, out-of-order `exp_level`, missing salary/employment, and an unrecognized extra query parameter (must preserve, not error). Tests for `summarizeExpLevels` cover the SC-004 verbatim shapes: `2y,3y,4y,5y → "2–5 years"`, `1y,2y,3y → "1–3 years"`, `1y,3y → "1, 3 years"`, `2y,2y → "2 years"`, `3y,1y,2y → "1–3 years"`, empty → `""`, one value → `"3 years"`, non-integer `senior` → `"senior"` (forced list fallback).
- [X] T007 [P] [US2] Port `summarizeExpLevels(values: string[]): string` to `apps/dashboard/src/features/sources/djinniSearchSummary.ts` — same logic, same output for same input, same SC-004 shapes — per `contracts/djinni-url-shapes.md` §4. Pure function, no React/DOM, no module side effects.
- [X] T008 [P] [US2] Add vitest tests `apps/dashboard/src/features/sources/djinniSearchSummary.test.ts` covering the same SC-004 cases listed for T006, plus the en-dash character sanity check (`"2–5 years"` contains `\u2013`, not `-`).

**Checkpoint**: Foundation ready — `DjinniDetect` recognizes both URL shapes, `ParseBasicSearch` returns the filter set, and the shared `summarizeExpLevels` rule is implemented + tested in both Go and TS. `go test ./apps/api/internal/jobsources/adapters/...` and `pnpm --filter @job-finder/dashboard test` both green. User-story implementation can begin in parallel.

---

## Phase 3: User Story 1 - Save and run a basic-search URL (Priority: P1) 🎯 MVP

**Goal**: An operator can paste `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote` (and the single-page Golang example URL) as a Djinni subscription, the system saves it, runs it, paginates correctly through one or many pages, and ingests listings into the feed.

**Independent Test**: Run quickstart §1 (save both example URLs; reject neither-shape URLs) and §3 + §4 (run the single-page Golang search → completes after page 1 with `succeeded` outcome, no loop; run the multi-page Node.js search → listings appear in feed; re-run → zero new listings, 100% dedup). Covers spec acceptance scenarios US1.1–US1.5 and SC-001/SC-002/SC-005/SC-007.

### Tests for User Story 1

- [X] T009 [P] [US1] Add `TestValidateDjinniSubscriptionURL` table-driven test in `apps/api/internal/subscriptions/service_test.go` covering: dashboard URL accepted, basic-search URL (all filters) accepted, basic-search URL (only `search_type`+`primary_keyword`) accepted, basic-search URL with `page=N` already in saved URL accepted, neither-shape URL rejected with reason, non-`djinni.co` host rejected with reason, single-job-posting path `/jobs/<id>` without `search_type=basic-search` rejected with reason, `/companies/...` rejected with reason. Each test asserts accept → `nil`, reject → non-nil error with a substring of the human-readable reason.
- [X] T010 [P] [US1] Extend `apps/api/internal/jobsources/adapters/djinni_test.go` with a `TestDjinniSearchBasicSearchSinglePage` that serves a single-page fixture (via `httptest.Server`) of a `/jobs/?search_type=basic-search&...` URL, plus an empty page-2 response, runs `adapter.Search(ctx, query, config)` with `query.SubscriptionURL` set to that basic-search URL, and asserts: (a) the run returned the page-1 cards, (b) only one fetch to `/jobs/...` and one fetch to `&page=2` (verified via the test server's request counter), (c) no error. Tests the single-page guard from research.md R1 / SC-002.
- [X] T011 [P] [US1] Extend `apps/api/internal/jobsources/adapters/djinni_test.go` with a `TestDjinniSearchBasicSearchMultiPage` serving two non-empty pages then an empty page-3, asserting: (a) all page-1+page-2 cards returned, (b) page-3 fetched then loop broken on empty, (c) `seenFirstHref == cards[0].URL` redirect-loop guard is honor-correct: include a separate `TestDjinniSearchBasicSearchRedirectLoop` where page-2 returns the same first card as page-1 and confirm the run stops after 2 fetches. No error in either case.
- [X] T012 [P] [US1] Extend `apps/api/internal/jobsources/adapters/djinni_test.go` with a `TestDjinniSearchBasicSearchPreservesQueryParams` asserting that the request URL issued by `adapter.Search` carries `search_type=basic-search`, `primary_keyword`, `salary`, EVERY `exp_level` value (preserving duplicates and order), `employment`, and any unrecognized extra param present in the saved subscription URL. Use the test server's recorded request URL — assert via `url.Parse` + `Query()` rather than substring matching.
- [X] T013 [P] [US1] Extend `apps/api/internal/jobsources/adapters/djinni_test.go` with a `TestDjinniSearchBasicSearchStripsPageParamFromSavedURL` — saved URL `...&page=4` — asserting the run's first fetch uses `page=1`, not `page=4`. Maps to FR-003 and the spec edge case "Basic-search URL with `page=N` already present when saved".

### Implementation for User Story 1

- [X] T014 [US1] Add `case "djinni":` branch in `apps/api/internal/subscriptions/service.go`'s `validateSubscriptionURL` switch, calling a new `validateDjinniSubscriptionURL(rawURL string) error`. The new function uses `DjinniDetect` from T003: `Unknown` → reject with the human-readable reason from `contracts/djinni-url-shapes.md` §1/§3; `Dashboard` or `BasicSearch` → `nil`. Mirrors the existing validator precedent (`validateGlassdoorSubscriptionURL`, `validateWellfoundSubscriptionURL`) — pure `url.Parse`, no network, never returns an error for a valid dashboard URL that was already being accepted.
- [X] T015 [US1] Update `apps/api/internal/subscriptions/service_test.go` for any test that previously exercised the `default: nil` path for `case "djinni"` if such tests exist (search first; if no existing test asserts the old permissive behavior, skip this task). Keep `TestValidateDjinniSubscriptionURL` (T009) as the canonical coverage.
- [X] T016 [US1] Refactor `apps/api/internal/jobsources/adapters/djinni.go` `Search` to branch on `DjinniDetect(query.SubscriptionURL)` when `query.SubscriptionURL != ""`: `DjinniModeDashboard` → `scrapeDashboard`, `DjinniModeBasicSearch` → `scrapeBasicSearch`, `DjinniModeUnknown` → return a clear `fmt.Errorf("djinni subscription url is neither dashboard nor basic-search shape: %s", sub)` (defensive only — save-time validation already rejects Unknown per T014). When `query.SubscriptionURL == ""`, keep the existing inline `keywords`+`employment=remote` URL build path unchanged (it's the scheduler's `djinni.co/jobs/?primary_keyword=Golang` fixture path used by `apps/api/internal/ingestion/scheduler_test.go:193` — FR-013).
- [X] T017 [US1] Extract the existing `scrapeSubscription` pagination body in `apps/api/internal/jobsources/adapters/djinni.go` into a shared helper — name it `paginateDjinni(ctx, firstPageURL, headers)` — that takes the URL of page 1 already constructed, then walks `page=N` using the fetch-doc/empty-page/first-card-stable/50-page-cap guards from research.md R1. `scrapeDashboard` (the renamed `scrapeSubscription`) calls it with the dashboard URL; no behavioral change to dashboard mode (FR-013, SC-008).
- [X] T018 [US1] Implement `scrapeBasicSearch` in `apps/api/internal/jobsources/adapters/djinni.go` to: (a) call `ParseBasicSearch` for logging only (`slog.Info("djinni: running basic-search", "filters", filters)`), (b) construct the first-page URL by parsing the saved URL, forcing `page=1` (overwrite any `page` param per FR-003 / T013), preserving every other query param, (c) delegate to `paginateDjinni` for the actual fetch loop. Anonymous-operation is honored automatically — `authHeaders(ctx)` returns empty headers when `D.Session == nil`, recyclable for both modes (research.md R6).
- [X] T019 [US1] Confirm — by reading `apps/api/internal/ingestion/handler.go` around line 117–140 — that no change is required there for basic-search subscriptions. The handler sets `query.SubscriptionURL = sub.Url` unconditionally and calls `adapter.Search(ctx, query, config)`. Add a one-line test in `apps/api/internal/ingestion/scheduler_test.go` if the existing `djinni.co/jobs/?primary_keyword=Golang` fixture is affected by the new URL-shape routing — it should NOT be, because that fixture exercises the `query.SubscriptionURL == ""` path which T016 leaves untouched.
- [X] T020 [US1] Optionally add one basic-search seed subscription to `apps/api/cmd/seed/subscriptions.go` mirroring the spec's Golang example URL, alongside the existing Djinni dashboard seed (if any). Marked optional per research.md R7 — keeps the diff minimal; only needed if an operator expects `make seed` to demonstrate both modes.
- [X] T021 [US1] Run `go test ./apps/api/...` and `make test-lint` (Go side only at this point — the TS side of US1 is empty by design, the dashboard is US2). Fix any failure introduced by the `djinni.go` refactor, especially leakage from `scrapeSubscription`'s old name into other call sites (search with grep before claiming done).

**Checkpoint**: User Story 1 is independently functional and testable. Quickstart §1 (accept both URLs, reject neither-shape), §2 (existing dashboard mode unchanged), §3 (single-page Golang run completes with `succeeded`), §4 (multi-page Node.js run + dedup), §7 (no-login run) all pass. US2 (display) is not yet touched; the Subscriptions list still shows the truncated raw `sub.url` for basic-search rows — that's the known US2 gap.

---

## Phase 4: User Story 2 - Display every basic-search filter on the subscription row (Priority: P2)

**Goal**: On the dashboard Subscriptions list, every query parameter present in a saved basic-search URL is rendered as part of the row label, with consecutive `exp_level` values collapsed to a range. Dashboard `subs/{id}/` URLs keep their existing label unchanged.

**Independent Test**: Run quickstart §5 — open Sources, observe Node.js sub row shows a summary of `Node.js` + salary + `"2–5 years"` + `remote`; Golang sub row shows `Golang` + salary + `"1–3 years"` + `remote`; a non-consecutive `1y,3y` test sub shows `"1, 3 years"`; a duplicate `2y,2y` test sub shows `"2 years"`; an out-of-order `3y,1y,2y` test sub shows `"1–3 years"`; the dashboard `subs/{id}/` row is unchanged from before the feature shipped (SC-008). Covers spec acceptance scenarios US2.1–US2.4 and SC-003/SC-004.

### Tests for User Story 2

- [X] T022 [P] [US2] Add vitest tests `apps/dashboard/src/features/sources/djinniSearchSummary.test.ts` (extended from T008) for `summarizeDjinniBasicSearch(url: string): string | null`. Cover: returns the rendered summary (with `Node.js`, `3000`, `"2–5 years"`, `remote`) for the Node.js example; same for the Golang example with `"1–3 years"`; omits absent filters cleanly (a URL with only `primary_keyword=Golang` returns a label containing `Golang` and no salary/levels/employment tokens); returns `null` for a dashboard `subs/{id}/` URL (so the row falls back to the existing default label per FR-013); returns `null` for a non-djinni.co URL (defensive — the server should never send one, but the client must not crash); returns `null` for an empty/garbage URL.
- [X] T023 [P] [US2] Add or extend a `SourcesPage` (render) test at `apps/dashboard/src/features/sources/SourcesPage.test.tsx` (if it doesn't exist, create it mirroring the convention used by `apps/dashboard/src/features/feed/FeedPage.test.tsx`) with a row snapshot/integration test: render `SubscriptionRow` for a basic-search subscription and assert the rendered DOM contains (a) every expected filter fragment, (b) the `"2–5 years"` range string and NOT a comma-separated `2y, 3y, ...` list, (c) no `$null` / `undefined` / `"null"` placeholder for a URL missing salary. Then render the same for a dashboard-mode subscription and assert the existing default label is unchanged — covers SC-008.

### Implementation for User Story 2

- [X] T024 [US2] Implement `summarizeDjinniBasicSearch(url: string): string | null` in `apps/dashboard/src/features/sources/djinniSearchSummary.ts` — port `DjinniDetect` + `ParseBasicSearch` + `summarizeExpLevels` (T007) into one pure function. Returns `null` when `DjinniDetect(url) !== 'basic-search'`. Renders the present filters in a stable order — `primary_keyword`, then `salary` (as `$<n>`), then `exp_level` set (via `summarizeExpLevels`), then `employment` — joined by ` · ` (with surrounding spaces). Omit any absent filter from the output entirely (no placeholder) per FR-012. The ordering and the ` · ` delimiter match the example in `data-model.md` and the contracts file §4.
- [X] T025 [US2] Wire `summarizeDjinniBasicSearch` into `SubscriptionRow` in `apps/dashboard/src/features/sources/SourcesPage.tsx`. When `sub.sourceKey === 'djinni'`, call the helper with `sub.url`; if it returns a non-null string, render that summary as the row's primary label in place of the existing `sub.name ?? sub.sourceKey` line (the `sub.sourceKey` chip in the row stays as a small "djinni" badge — that's what visually distinguishes the two Djinni modes from each other since the dashboard-mode row keeps `sub.name ?? sub.sourceKey` per FR-013). If the helper returns `null`, leave the row exactly as it renders today — the existing truncated-`sub.url` subtitle line remains as the secondary identifier. Do not add a new `SubscriptionDto` field; do not change the `SubscriptionInput` shape.
- [X] T026 [US2] Verify (via `grep -n 'SubscriptionDto' packages/shared/src/`) that no `SubscriptionDto` field was added or changed — the TS port relies on the existing `url: string` field only. Run `pnpm --filter @job-finder/dashboard typecheck` and `pnpm --filter @job-finder/dashboard test`. If `make test-lint` triggers a `packages/shared` rebuild, it runs cleanly without any `dto.go` / `index.ts` change required (research.md R3).

**Checkpoint**: User Stories 1 AND 2 now both work independently. Quickstart §1–§5 all pass. The given Node.js and Golang example URLs are saved, run, paginated correctly (single-page and multi-page), deduplicated, and rendered with range-collapsed `exp_level` display — the full core request from the user description is delivered at this MVP checkpoint.

---

## Phase 5: User Story 3 - Distinguish the two Djinni search modes (Priority: P3)

**Goal**: Beyond the URL-shape discrimination already used by US1 (run) and US2 (display), make sure the two modes are clearly separable in logs, run records, and the UI so an operator can tell at a glance which mode a saved Djinni subscription is, and so a URL saved under one mode is never silently interpreted as the other.

**Independent Test**: Run quickstart §5 — a basic-search row's label includes the rendered filter summary (US2) plus the existing `"djinni"` chip; a dashboard `subs/{id}/` row shows the existing default label (FR-013, SC-008). Run quickstart §2 — saving a dashboard URL still goes through `scrapeDashboard` unchanged; saving a basic-search URL goes through `scrapeBasicSearch`. Inspect `apps/api` logs and confirm a run's log entry includes the mode name (e.g. `scrapeBasicSearch` / `scrapeDashboard`). Covers spec acceptance scenarios US3.1–US3.3 and SC-006/SC-008.

### Tests for User Story 3

- [X] T027 [P] [US3] Extend `apps/api/internal/jobsources/adapters/djinni_test.go` with `TestDjinniSearchDashboardModeUnchanged` — serve a `/my/dashboard/subs/123/` fixture set and assert the run uses `scrapeDashboard` (verifiable by checking the test server only sees the dashboard path prefix, never `/jobs/?search_type=...`). Cements FR-013/SC-008: the dashboard mode's behavior is verbatim unchanged after the refactor.
- [X] T028 [P] [US3] Extend `apps/dashboard/src/features/sources/SourcesPage.test.tsx` (created in T023) with an explicit assertion that a dashboard-mode row's label contains neither a salary fragment nor an `exp_level` range fragment, while a basic-search row's label contains both — i.e., the two modes produce visually distinct row labels, satisfying FR-011 and the US3 visual-distinguishability acceptance.

### Implementation for User Story 3

- [X] T029 [US3] Audit `apps/api/internal/jobsources/adapters/djinni.go` log lines introduced by T018 / `scrapeBasicSearch` and the renamed `scrapeDashboard` (T017): ensure each run's log entry names the mode (`slog.Info("djinni: running basic-search", ...)` and `slog.Info("djinni: running dashboard subscription", ...)`), so run records / logs are filterable by mode — covers US3's "tell at a glance which mode" requirement on the backend.
- [X] T030 [US3] Add a small mode marker on the dashboard's `SubscriptionRow` — e.g. render the existing `"djinni"` chip and append a tiny `· basic-search` or `· dashboard` sub-text when `sub.sourceKey === 'djinni'` and the helper T024 was used (non-null) or returned null respectively — under the row's primary label, using the existing `text-xs text-faint` styling. This is a UI-only addition; no data change, no contract change.
- [X] T031 [US3] Run `make test-lint` and confirm both US3 sub-tests pass (T027 on Go side, T028 on TS side). No new endpoints, no migrations.

**Checkpoint**: All three user stories are independently functional. US1 (run), US2 (display filters with range), US3 (mode distinguishability on backend logs + frontend row marker) all pass their independent tests.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cross-app quality gate and end-to-end validation per the constitution and the quickstart.

- [X] T032 [P] Run `make test-lint` from repo root. This triggers the Go suite (`go test ./...`), the React suite (`vitest`), lint, and any cross-app checks required by constitution IV. Fix any failure in either app before declaring done — this feature crosses the Go/TS boundary and therefore MUST clear the cross-app gate (constitution IV).
- [X] T033 [P] Run quickstart §1 through §8 from `specs/015-djinni-basic-search-mode/quickstart.md` end-to-end against a local `make up` stack. Capture any mismatch against the expected outcomes (single-page stop, multi-page pagination + dedup, all five SC-004 `exp_level` shapes, no-login run, block reporting). Do not mark the feature done until all eight steps produce their expected outcomes.
- [X] T034 Review `apps/api/internal/jobsources/adapters/djinni.go` and `apps/api/internal/subscriptions/service.go` after the refactor for naming consistency — e.g., the old name `scrapeSubscription` only appears in the renamed `paginateDjinni`'s callers' comments and not as a dead stub. Remove any dead code or leftover comment references to the pre-refactor naming.
- [X] T035 [P] If `apps/api/cmd/seed/subscriptions.go` was updated in T020, verify `make seed` runs cleanly and the Sources screen shows the two Djinni seed subscriptions with their full US2 labels + US3 mode markers. If T020 was skipped (research.md R7 optional), skip this task.
- [X] T036 Commit using the existing repo convention (`feat:` prefix — see prior `feat(sources): ...` style in `git log`) with a message capturing the new basic-search mode + display summary; commit only the files changed (the two new Go files, the two new TS files, the modified `djinni.go`, `service.go`, `SourcesPage.tsx`, optional `subscriptions.go`, and the new test files). Do not include unrelated changes; do not commit any secret or locally-saved fixture HTML.

**Checkpoint**: Feature is complete. `make test-lint` passes. The full quickstart validation passes. Ready for review.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — T001 and T002 run in parallel.
- **Foundational (Phase 2)**: Depends on Phase 1 files existing. T003 → T004 → T005 within the single new Go file (sequential writes to the same file). T006 depends on T003+T004+T005. T007 and T008 are independent of Go and run in parallel with each other (and with the Go foundation once Phase 1 lands).
- **User Stories (Phase 3+)**:
  - US1 (Phase 3) depends on T003, T005, T006 (foundation) and T009–T013 (its own tests can be written first, in parallel, because they target separate test files).
  - US2 (Phase 4) depends on T007, T008 (foundation) and can run in parallel with US1 — different files, no shared touchpoint (`SubscriptionRow` modification T025 only touches `SourcesPage.tsx`, which US1 does not touch). US2 also depends on US1 having saved a basic-search URL to display in tests; coordinate by having US2 tests use synthetic fixtures rather than depending on US1's live seed.
  - US3 (Phase 5) depends on US1 (T017, T018) and US2 (T025) being complete — its audit (T029) and UI marker (T030) reference the names and labels those tasks introduced.
- **Polish (Phase 6)**: Depends on US1–US3 being complete.

### User Story Dependencies

- **US1 (P1) – MVP**: Can start after Phase 2 Go foundation (T003–T006). Independent of US2/US3; that's why it's the MVP.
- **US2 (P2)**: Can start after Phase 2 TS foundation (T007–T008). No dependency on US1 within the implementation, only within end-to-end validation (a basic-search URL must exist before US2 has something to render — use synthetic fixtures in unit tests, real saved subscription only for the quickstart integration step).
- **US3 (P3)**: Depends on US1 (mode names in `scrapeBasicSearch`/`scrapeDashboard`) and US2 (mode-aware display) being merged. Pure audit+marker work; small surface.

### Within Each User Story

- Tests target new or existing test files only; they do NOT require the implementation to exist first when following TDD (write tests to fail first for T009/T013/T023).
- Implementation tasks within a story may share a single Go file (`djinni.go` for US1 — sequential to avoid same-file conflict).
- Story is complete only when both its tests AND implementation tasks are green AND its quickstart independent-test criteria pass.

### Parallel Opportunities

- Phase 1: T001 ‖ T002 (different files).
- Phase 2: Go cluster (T003 → T004 → T005 → T006, same file — sequential) runs in parallel with TS cluster (T007 ‖ T008, two new files) — different stacks, different files, no conflict.
- Phase 3 (US1): T009, T010, T011, T012, T013 (test files) can all run in parallel because they target separate test cases within the same suite file — synchronization is needed only when writing to `djinni_test.go`; in practice, batch them. T014 (subscriptions validator) ‖ T016/T017/T018 (`djinni.go`) — different files, can run in parallel until T018 needs the validator's `DjinniDetect` export (already in foundation, T003).
- Phase 4 (US2): T022 ‖ T023 (separate test files) ‖ implementation T024, T025 — but T025 depends on T024 existing in the module first.
- Phase 5 (US3): T027 ‖ T028 (different stacks).

With two developers, one picks the Go lane (US1 → US3-Go audit) and the other picks the TS lane (US2 → US3-TS marker); US3's final polish (T032 cross-app `make test-lint`) needs both lanes merged.

---

## Parallel Example: User Story 1

```bash
# Once Phase 2 foundation is in, launch US1's test-writing tasks in parallel
# (they target distinct subsections of djinni_test.go and a distinct
# file for the validator, so structure each as its own logical commit
# within the same test file):
Task: "T009 TestValidateDjinniSubscriptionURL in apps/api/internal/subscriptions/service_test.go"
Task: "T010 TestDjinniSearchBasicSearchSinglePage in apps/api/internal/jobsources/adapters/djinni_test.go"
Task: "T011 TestDjinniSearchBasicSearchMultiPage + RedirectLoop (same file, different test functions — can batch)"
Task: "T012 TestDjinniSearchBasicSearchPreservesQueryParams (same file)"
Task: "T013 TestDjinniSearchBasicSearchStripsPageParamFromSavedURL (same file)"

# Then the implementation tasks, sequentially (they share djinni.go):
Task: "T014 add validateDjinniSubscriptionURL in apps/api/internal/subscriptions/service.go"
Task: "T016 route Search() by DjinniDetect in djinni.go"
Task: "T017 extract paginateDjinni shared helper in djinni.go"
Task: "T018 implement scrapeBasicSearch in djinni.go"
```

---

## Parallel Example: User Story 2 (runs in tandem with US1 if two developers)

```bash
# Tests first, in parallel (different files, no shared state):
Task: "T022 vitest tests for summarizeDjinniBasicSearch in djinniSearchSummary.test.ts"
Task: "T023 SourcesPage SnapshotRow/display test in SourcesPage.test.tsx"

# Implementation (T024 then T025 in the same TS file/module):
Task: "T024 summarizeDjinniBasicSearch implementation in djinniSearchSummary.ts"
Task: "T025 wire SubscriptionRow to use the helper in SourcesPage.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup — create the two file skeletons (T001 ‖ T002).
2. Phase 2: Foundational — Go URL-shape discriminator + `exp_level` rule (T003–T006), TS mirror (T007–T008).
3. Phase 3: User Story 1 — validate at save, route in `Search`, paginate via shared helper + `scrapeBasicSearch`, all fixture tests pass.
4. **STOP and VALIDATE**: run quickstart §1–§4 + §7. The user's literal two example URLs (Node.js 2y–5y multi-page, Golang 1y–3y single-page) should save, run, paginate, deduplicate, and complete cleanly without a login configured — that's the literal request the user made. Display (US2) is still the truncated raw URL; acceptable for an internal MVP slice.

### Incremental Delivery

1. Setup + Foundational → shared contract + pure helpers in both apps.
2. Add User Story 1 → save/run/paginate/dedup end-to-end → validate (MVP slice, fully matches the user's "created one more search mode" and "some searches have only one page" requirements).
3. Add User Story 2 → display every filter on the row, `exp_level` collapsing to range → validate (matches the user's "all query params from url should be displayed on frontend" + "years if its sequence displayed as range" requirement).
4. Add User Story 3 → cross-app mode distinguishability markers + backend log mode names → validate.
5. Polish → `make test-lint`, full quickstart §1–§8, clean commit.

### Parallel Team Strategy

With two developers after Phase 2:

- Developer A takes the Go lane: US1 implementation → T014, T016, T017, T018 → US3 T027, T029 → T021, T031.
- Developer B takes the TS lane: US2 implementation → T024, T025 → US3 T028, T030 → T026.
- Merge often; the only true cross-lane sync is the T032 `make test-lint` run at the end of US3 / Phase 6 polish.

---

## Notes

- [P] tasks = different files, no dependencies.
- [Story] label maps each task to its user story for traceability.
- Each user story is independently completable and testable per the spec's quickstart independent-test criterion.
- Tests are included (not optional) because constitution IV makes per-language test suites mandatory, and the shared `summarizeExpLevels` rule needs tests on BOTH sides to prove the cross-app contract holds.
- Verify tests fail before implementing when following TDD — T009, T010, T022, T023 are designed to fail first because they assert new behavior not present yet.
- Commit after each task or logical group, using `feat:` prefix per the repo convention.
- Stop at any checkpoint to validate the story independently — the US1 checkpoint is the literal user request (basic-search save + run + single-page pagination).
- Avoid: vague tasks (each task names an exact file path and a behavioral assertion), same-file conflicts (US1's three `djinni.go` writes are sequential, not parallel), cross-story dependencies that break independence (the only real cross-story dependency is US3 referencing names+labels from US1/US2 — that's flagged explicitly in US3's prerequisites).