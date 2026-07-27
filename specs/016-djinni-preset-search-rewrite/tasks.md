---

description: "Task list for Djinni Preset-Search Rewrite"
---

# Tasks: Djinni Preset-Search Rewrite

**Input**: Design documents from `/specs/016-djinni-preset-search-rewrite/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/README.md, quickstart.md

**Tests**: No TDD requested. Existing v015 tests for basic-search/pagination/detect/parse are KEPT (they guard the contract the rewrite preserves); session/login/dashboard test cases are PRUNED as part of the deletion tasks. Test pruning is implementation work, not test-first authoring.

**Organization**: Tasks grouped by user story. This rewrite is mostly deletion — the "foundational" phase removes the legacy session/login/dashboard code that blocks the anonymous preset path (US1); US3 phase owns the data migration + final static verification that the legacy is gone.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g. US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Monorepo: `apps/api/` (Go backend), `apps/dashboard/` (React/Vite), `packages/shared/` (TS types — NOT TOUCHED). Paths are repo-relative.

---

## Phase 1: Setup

**Purpose**: No project initialization needed (existing monorepo). This phase is intentionally empty — see [plan.md](./plan.md) Project Structure: the only new file is the goose migration, created in US3.

- [X] T001 Confirm local stack is healthy: `make up` then `make test-lint` on `main` as a green baseline before editing (per constitution Principle IV)

---

## Phase 2: Foundational (Blocking Prerequisites — Legacy Code Deletion)

**Purpose**: Delete the session/login/dashboard code paths that block the anonymous preset-fetch path (US1). These MUST complete before US1 because the adapter cannot compile/use anonymous fetch while the `Session` field and `UsesUserAccount()` remain.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete. All deletions here are code-only (no DB migration — that's US3).

- [X] T002 Delete whole file `apps/api/internal/jobsources/adapters/djinni_session.go` (removes `DjinniSession`, `DjinniSessionProvider`, `DjinniConfigStore`, `djinniLogin`, `djinniCSRFToken`, `djinniBaseURL`, `djinniUserAgent`)
- [X] T003 [P] Remove `DjinniEmail` and `DjinniPassword` struct fields and their `mapstructure` tags from `apps/api/internal/config/config.go` (around lines 97-98) — keep `DjinniDetailDelayMs`
- [X] T004 [P] Remove `"DJINNI_EMAIL"` and `"DJINNI_PASSWORD"` entries from the optional-keys list in `apps/api/internal/config/defaults.go` (line 35) — keep `JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD` on the same line
- [X] T005 [P] Remove the `DJINNI_EMAIL` and `DJINNI_PASSWORD` lines from `.env.example` (lines ~70-71) — keep the `DJINNI_DETAIL_DELAY_MS=1500` line and reword its comment to drop "authenticated djinni account"
- [X] T006 [P] Audit `apps/api/internal/config/config_test.go` for any enumerated-keys assertion listing `DJINNI_EMAIL`/`DJINNI_PASSWORD`; if present, remove them from the expected set (line ~153 lists `DJINNI_DETAIL_DELAY_MS` — leave that, prune only the email/password entries if any)
- [X] T007 Remove `Platform.DjinniSession` field declaration and its construction from `cfg.DjinniEmail`/`cfg.DjinniPassword` in `apps/api/cmd/server/platform.go` (field at ~line 45, construction at ~line 110) — keep `Platform.JobLeadsSession` (line ~49)
- [X] T008 In `apps/api/cmd/server/compose_sources.go`: drop the `Session:` arg from the `adapters.DjinniAdapter{Scraping: p.Scraping, Session: …}` construction (line ~12) and delete the line `p.DjinniSession.Sources = sourcesSvc` (line ~46) — keep the `p.JobLeadsSession.Sources = sourcesSvc` line (~47) verbatim
- [X] T009 In `apps/api/internal/jobsources/adapters/djinni.go`: remove the `Session` field from `DjinniAdapter` (line ~31); delete the `authHeaders` method (~37-48), `setDjinniCookie` (~50-56), the `fetchDoc` relogin branch (~62-87) and auth bits of `fetchParse` (~89-95); delete `UsesUserAccount` (~107); remove the `DjinniModeDashboard` switch arm in `Search` (~121-122) and the `scrapeDashboard` function (~194-201); delete `djinniIsLoginPage` (~231). Keep `paginateDjinni`, `scrapeBasicSearch`, `parseDjinniCards`, `DjinniDetailPatch`, `HealthCheck`, `firstNonEmpty`, `djinniMaxSubscriptionPages`, `djinniRemoteRe`
- [X] T010 [P] In `apps/api/internal/jobsources/adapters/djinni_searchmode.go`: remove the `DjinniModeDashboard` const (line ~24) and the dashboard accept-branch of `djinniDetectShape` (lines ~50-53) — keep `DjinniModeUnknown`, `DjinniModeBasicSearch`, `DjinniDetect`, `BasicSearchFilters`, `ParseBasicSearch`, `summarizeExpLevels` verbatim
- [X] T011 Prune session/login/dashboard test cases from `apps/api/internal/jobsources/adapters/djinni_test.go`: delete `djinniLoginServer`, `fakeConfigStore`, `TestDjinniSession*`, `TestDjinniLogin*`, `TestDjinniIsLoginPage`, `TestDjinniSearchDashboardModeUnchanged`, and any basic-search test assertions that check `setDjinniCookie` was called — KEEP every `TestDjinniSearchBasicSearch*` (single-page, multi-page, loop-guard, param-preserve, page-strip)
- [X] T012 [P] Prune the dashboard-detect cases of `TestDjinniDetect` from `apps/api/internal/jobsources/adapters/djinni_searchmode_test.go` (the `subs/{id}/` cases) — KEEP `TestParseBasicSearch` and `TestSummarizeExpLevels` verbatim
- [X] T013 Run `go build ./apps/api/...` and `go test ./apps/api/internal/jobsources/adapters/ -run 'TestDjinni'` to confirm the deleted code compiles and the kept tests pass at this checkpoint

**Checkpoint**: Adapter compiles anonymous (no `Session` field, no `UsesUserAccount`); `DjinniAdapter{Scraping: ...}` is the new shape. Preset-search tests still pass. User story work can begin.

---

## Phase 3: User Story 1 - Run a Djinni preset search URL end-to-end with pagination (Priority: P1) 🎯 MVP

**Goal**: A saved preset-search URL runs anonymously through pagination (single-page and multi-page), reusing the shared fetch/pipeline, with `subs/{id}/` rejected at save time.

**Independent Test**: Save the spec's Golang preset URL (`?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote`), trigger a manual run with no `DJINNI_EMAIL`/`DJINNI_PASSWORD` set, and confirm Djinni listings appear in the feed with `SourceRun` status `ok` and exactly 2 `FetchHTML` calls for the single-page case (per [quickstart.md](./quickstart.md) Scenario 1 & 3).

### Implementation for User Story 1

- [X] T014 [US1] Anonymize `FetchDetail` in `apps/api/internal/jobsources/adapters/djinni.go` (~303-336): replace the `authHeaders`/`fetchDoc` relogin call with a direct `d.Scraping.FetchHTML(ctx, url, nil)` + `goquery` parse; keep `DjinniDetailPatch` extraction and `UpdateJobDetail`-bound return shape unchanged
- [X] T015 [US1] Collapse `validateDjinniSubscriptionURL` in `apps/api/internal/subscriptions/service.go` (lines ~260-282) to a single accept branch: host `djinni.co`/`www.djinni.co`, path `/jobs` or `/jobs/`, query `search_type=basic-search` present. Reject all other djinni.co URLs — including `/my/dashboard/subs/{id}/` — with the human-readable reason from [contracts/README.md](./contracts/README.md) C1: "Djinni subscriptions support only preset-search URLs (`djinni.co/jobs/?search_type=basic-search&…`); dashboard URLs are no longer supported."
- [X] T016 [US1] In `apps/api/internal/subscriptions/service.go` `TestValidateDjinniSubscriptionURL` (or equivalent): update/add cases asserting (a) preset URL accepted, (b) `subs/42/` rejected with the new reason, (c) `/jobs/123` single-posting rejected, (d) missing `search_type` rejected, (e) preset URL with unrecognized extra param (`&foo=bar`) accepted
- [X] T017 [P] [US1] In `apps/api/internal/enrichment/handler.go` `enrichDjinni` (~144-179): drop the `sources.GetByKey`/`DecryptConfig` call for session config (~145-149) since no session exists — pass `nil` config (or drop the `config` arg if the signature allows) to `h.djinni.FetchDetail`; keep the `case "djinni"` dispatch and the `NeedsDetail=true` enrich flow
- [X] T018 [US1] Verify `Search` in `apps/api/internal/jobsources/adapters/djinni.go` now has a single preset branch (no dashboard arm): `SubscriptionURL != ""` → `scrapeBasicSearch` + `paginateDjinni`; `SubscriptionURL == ""` → existing keyword fallback (`djinni.go` ~130-148) stays unchanged per [research.md](./research.md) R2 (saved-search targets in `seed/savedsearch.go:34` depend on it); `DjinniModeUnknown` returns a defensive error with the collapsed message
- [X] T019 [US1] Run `go test ./apps/api/internal/jobsources/adapters/ -run 'TestDjinniSearchBasicSearch'` and `go test ./apps/api/internal/subscriptions/ -run 'TestValidateDjinniSubscriptionURL'`; verify the single-page test asserts exactly 2 fetches and 2 cards, the multi-page test walks pages, and the loop-guard test stops on the repeated first-card href (per [quickstart.md](./quickstart.md) Scenarios 1-3)
- [ ] T020 [US1] End-to-end manual validation per [quickstart.md](./quickstart.md) Scenario 1: `make run-backend`, save the Golang preset URL via the dashboard Sources screen, trigger a manual run, confirm `ok` verdict and jobs in feed within 5 minutes — confirm no `POST /my/login/...` call or cookie-set appears in backend logs (anonymous path, FR-005)

**Checkpoint**: User Story 1 fully functional — preset URL saves, runs anonymously, paginates, and `subs/{id}/` is rejected at save time.

---

## Phase 4: User Story 2 - See every preset filter on the subscription in the dashboard (Priority: P2)

**Goal**: Each saved preset URL renders a human-readable filter summary (keyword · $salary · expSummary · employment) in the Subscriptions list, with consecutive `exp_level` values collapsed to a range and absent filters omitted.

**Independent Test**: Save the Node.js (exp 2y-5y) and Golang (exp 1y-3y) preset URLs and confirm each row's label reads `Node.js · $3000 · 2–5 years · remote` and `Golang · $1500 · 1–3 years · remote` respectively, with the en-dash `–` (U+2013), not a hyphen (per [quickstart.md](./quickstart.md) Scenario 7 and [contracts/README.md](./contracts/README.md) C4).

### Implementation for User Story 2

- [X] T021 [US2] Update the Djinni entry's `placeholder` in `apps/dashboard/src/features/sources/SourcesPage.tsx` (line ~322) from `https://djinni.co/my/dashboard/subs/{id}/` to the preset example `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote`
- [X] T022 [US2] Collapse `djinniModeMarker` in `apps/dashboard/src/features/sources/SourcesPage.tsx` (lines ~422-427): remove the `· dashboard` branch (unreachable post-migration per [research.md](./research.md) R7) and keep only the `· basic-search` marker when `basicSearchLabel !== null`; leave the `null` fallback (raw `url` display) intact for defensive display
- [X] T023 [US2] Confirm `apps/dashboard/src/features/sources/djinniSearchSummary.ts` and `djinniSearchSummary.test.ts` require NO edit (they are the v015-tested contract per [research.md](./research.md) R7) — run `pnpm --filter @job-finder/dashboard test -- src/features/sources/djinniSearchSummary.test.ts` and verify all cases pass (range-vs-list, en-dash, dedup, missing-filter omission, non-`Ny` fallback, and the `subs/{id}/` → `null` "not a preset URL" case)
- [ ] T024 [US2] Manual validation per [quickstart.md](./quickstart.md) Scenario 7: save the 6 example preset URLs in the dashboard and verify each row label matches the C4 contract exactly (including out-of-order consecutive `1–3 years` and duplicate `2 years`)

**Checkpoint**: User Story 2 renders preset summaries correctly; the dashboard branch is gone.

---

## Phase 5: User Story 3 - All legacy Djinni paths are removed (Priority: P3)

**Goal**: Pre-existing `subs/{id}/` subscriptions are deleted via a one-time goose migration with an audit trail, and static verification confirms zero references to `subs/{id}/`, session login, or dual-mode dispatch remain in the codebase.

**Independent Test**: Seed a `subs/{id}/` subscription with the old code, apply the rewrite + migration `00027`, and confirm (a) the `subs/42/` row is gone from `Subscription`, (b) one audit row exists in `DjinniLegacySubAudit`, (c) preset subscriptions are untouched, (d) the static greps in [quickstart.md](./quickstart.md) Scenario 10 return zero matches (per [quickstart.md](./quickstart.md) Scenarios 5 & 10 and [contracts/README.md](./contracts/README.md) C3).

### Implementation for User Story 3

- [X] T025 [US3] Create `apps/api/internal/db/migrations/00027_drop_djinni_dashboard_subs.sql` (next sequential number after `00026_host_retrieval_state.sql` per constitution's unique-sequential rule): `-- +goose Up` creates `DjinniLegacySubAudit` (id UUID PK, `subscriptionId` UUID — non-FK, `name` TEXT, `url` TEXT, `deletedAt` TIMESTAMPTZ) then `DELETE FROM "Subscription" WHERE "sourceKey" = 'djinni' AND "url" LIKE '%/my/dashboard/subs/%' RETURNING "id","name","url","createdAt"` and inserts each returned row into the audit table with `deletedAt = now()`; `-- +goose Down` is a no-op with a comment "deletion is irreversible — audit rows remain as the record". See [data-model.md](./data-model.md) §3 and [contracts/README.md](./contracts/README.md) C3
- [X] T026 [US3] Fix the seeded Djinni subscription URL in `apps/api/internal/seed/subscriptions.go` (lines ~18-23): replace the invalid `https://djinni.io/jobs?technology=golang&remote=true` (wrong host, bypasses validation per [research.md](./research.md) R2) with the spec's preset example `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote` — leave `seedSubscriptions`'s CreateSubscription call shape unchanged
- [ ] T027 [US3] Integration test for the migration per [quickstart.md](./quickstart.md) Scenario 5: with the old code, seed a `https://djinni.co/my/dashboard/subs/42/` subscription; apply migration `00027`; assert (a) the row is gone from `Subscription`, (b) one row exists in `DjinniLegacySubAudit` with `url='https://djinni.co/my/dashboard/subs/42/'`, (c) a preset `Subscription` row is untouched, (d) `JobSource` row with `key='djinni'` still exists. If the repo has no migration-test harness, validate manually via the `psql` queries in [quickstart.md](./quickstart.md) Scenario 5
- [X] T028 [US3] Run the static verification greps from [quickstart.md](./quickstart.md) Scenario 10 and confirm ALL return zero matches: `test ! -f apps/api/internal/jobsources/adapters/djinni_session.go`; `! grep -n "DjinniModeDashboard\|scrapeDashboard" apps/api/internal/jobsources/adapters/djinni.go apps/api/internal/jobsources/adapters/djinni_searchmode.go`; `! grep -rn "DJINNI_EMAIL\|DJINNI_PASSWORD" apps/api/ .env.example`; `! grep -n "Platform.DjinniSession" apps/api/cmd/server/platform.go apps/api/cmd/server/compose_sources.go`; `! grep -n "UsesUserAccount" apps/api/internal/jobsources/adapters/djinni.go`; `! grep -n "sessionCookie" apps/api/internal/jobsources/adapters/djinni.go`

**Checkpoint**: User Story 3 complete — legacy data pruned with audit trail, codebase verified legacy-free.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cross-app quality gate and end-to-end validation.

- [X] T029 Run `make test-lint` (Go + Vitest + lint) per constitution Principle IV — this is the cross-app gate (the change touches `apps/api`, `apps/dashboard`, and a migration). Confirm green
- [ ] T030 Run the full [quickstart.md](./quickstart.md) validation pass: Scenarios 1-10 (save/run preset, single-page, multi-page, save-time rejection, legacy migration, no-login, display labels, failure posture, non-Djinni sources unchanged, static greps)
- [X] T031 [P] Reword the `DjinniDetailDelayMs` comment in `apps/api/internal/config/config.go` (lines ~104-106) to drop "authenticated djinni account" wording (kept for pacing per FR-016; comment-only change)
- [ ] T032 [P] Run `make migrate-up` on a clean local DB and confirm idempotency: re-applying `00027` on a clean DB creates an empty audit table and deletes zero rows (per [contracts/README.md](./contracts/README.md) C3 idempotency clause)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — `make test-lint` green baseline.
- **Foundational (Phase 2)**: Depends on Phase 1 — BLOCKS all user stories. Code-only deletions.
- **US1 (Phase 3)**: Depends on Phase 2 (anonymous adapter shape is prerequisite for the preset run).
- **US2 (Phase 4)**: Depends on Phase 2 (dashboard marker collapse assumes the dashboard branch is unreachable). Independent of US1 — can run in parallel with US1 if a second developer is available.
- **US3 (Phase 5)**: The migration depends on Phase 2 (the validator must reject `subs/{id}/` before the migration deletes the rows, so re-saving them is impossible). The static greps depend on Phase 2 + US1 (the dashboard arm and `UsesUserAccount` are gone) and US2 (the dashboard marker collapse).
- **Polish (Phase 6)**: Depends on all user stories complete.

### User Story Dependencies

- **User Story 1 (P1, MVP)**: Phase 2 complete → US1 starts. No dependency on US2/US3.
- **User Story 2 (P2)**: Phase 2 complete → US2 starts. Independent of US1/US3 (different files: `SourcesPage.tsx` vs `djinni.go`/`service.go`).
- **User Story 3 (P3)**: Phase 2 + US1's validator collapse (T015) complete → US3 migration is safe. The static greps (T028) additionally require US2 (T022) to be complete.

### Within Each User Story

- Deletions before validator collapse (Foundational before US1).
- Validator collapse before migration (US1 before US3, so re-saving a pruned sub is impossible).
- Display edits independent (US2 parallelizable with US1 after Phase 2).
- Polish `make test-lint` only after all stories complete.

### Parallel Opportunities

- Phase 2: T003/T004/T005/T006/T010 touch distinct files (config, defaults, .env.example, config_test, djinni_searchmode) — all parallelizable. T002/T007/T008/T009/T011/T012 are sequential (T002 deletes the file others reference; T007/T008/T009 edit compose/wiring referencing the deleted symbols; T011/T012 prune tests).
- Phase 3: T017 (`enrichment/handler.go`) parallel with T014/T015/T016/T018/T019 (different file).
- Phase 4: T021/T022 sequential (both edit `SourcesPage.tsx`); T023/T024 are verification, after T021/T022.
- Phase 5: T025/T026 parallel (new migration file vs seed edit); T027/T028 are verification, after T025/T026.
- Phase 6: T031/T032 parallel (comment edit vs migration idempotency check); T029/T030 are final gates.

---

## Parallel Example: Phase 2 (Foundational Deletions)

```bash
# Parallel cluster 1 — distinct config/env files (no shared symbols):
Task: "T003 Remove DjinniEmail/DjinniPassword from apps/api/internal/config/config.go"
Task: "T004 Remove DJINNI_EMAIL/DJINNI_PASSWORD from apps/api/internal/config/defaults.go"
Task: "T005 Remove DJINNI_EMAIL/DJINNI_PASSWORD from .env.example"
Task: "T006 Audit apps/api/internal/config/config_test.go for enumerated-keys assertion"
Task: "T010 Drop DjinniModeDashboard from apps/api/internal/jobsources/adapters/djinni_searchmode.go"

# Sequential after cluster 1 — edits reference the deleted session symbols:
Task: "T002 Delete apps/api/internal/jobsources/adapters/djinni_session.go"
Task: "T007 Remove Platform.DjinniSession from apps/api/cmd/server/platform.go"
Task: "T008 Drop Session arg from DjinniAdapter construction in apps/api/cmd/server/compose_sources.go"
Task: "T009 Prune auth/dashboard methods from apps/api/internal/jobsources/adapters/djinni.go"
Task: "T011 Prune session/login/dashboard tests from apps/api/internal/jobsources/adapters/djinni_test.go"
Task: "T012 Prune dashboard-detect tests from apps/api/internal/jobsources/adapters/djinni_searchmode_test.go"
Task: "T013 go build + go test TestDjinni* checkpoint"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (baseline green).
2. Complete Phase 2: Foundational deletions (anonymous adapter shape).
3. Complete Phase 3: User Story 1 (preset run + validator collapse + anonymous `FetchDetail`).
4. **STOP and VALIDATE**: Save the Golang preset URL, run anonymously, confirm `ok` verdict + jobs in feed — that's the MVP.
5. The migration (US3) and display polish (US2) can ship in a follow-up.

### Incremental Delivery

1. Setup + Foundational → anonymous adapter shape.
2. Add User Story 1 → test independently → MVP shipped (preset search works).
3. Add User Story 2 → test independently → preset summaries render.
4. Add User Story 3 → test independently → legacy subs pruned + audited, codebase verified legacy-free.
5. Polish → `make test-lint` + full quickstart validation.

### Parallel Team Strategy

With two developers:
1. Both complete Phase 1 + Phase 2 together (deletions reference each other — coordinate).
2. Once Foundational done:
   - Developer A: User Story 1 (adapter + validator; backend).
   - Developer B: User Story 2 (dashboard display; frontend).
3. US3 (migration + static greps) after US1's validator collapse (T015) — single developer.
4. Polish (Phase 6) once all stories land.

---

## Notes

- `[P]` tasks = different files, no dependencies on incomplete tasks.
- `[Story]` label maps task to specific user story for traceability.
- No `tygo` regeneration (no `dto.go` change), no `sqlc-generate` (no query change), no `packages/shared` rebuild (no shared DTO change) — confirmed by [research.md](./research.md) R2.
- The rewrite is *mostly deletion* — Task IDs T002/T007/T008/T009 are deletions, not new code.
- The single new file is T025 (`00027_…sql`); all other tasks are edits to or deletions of existing Djinni-specific files.
- Commit after each task or logical group; conventional commit format per AGENTS.md (`feat:`, `fix:`, `chore:`, `docs:`).
- `make test-lint` is the gate before merge (constitution Principle IV — cross-app change).
- Verify the v015-kept tests fail-then-pass only if you touch their behavior; the rewrite preserves basic-search behavior verbatim, so those tests should pass throughout.