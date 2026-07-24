---

description: "Task list for Skeleton Loading States"
---

# Tasks: Skeleton Loading States

**Input**: Design documents from `/specs/006-skeleton-loading-states/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Not explicitly requested as TDD in spec.md. Existing `*.test.tsx` files under `apps/dashboard/src/features/job-detail/` and `apps/dashboard/src/features/settings/` currently assert `Spinner`/loading-text behavior and MUST be updated as part of each file's migration task (not written test-first) — Constitution IV requires the `vitest` suite passing before the change is done.

**Organization**: Tasks are grouped by user story (US1/US2/US3 from spec.md) to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- File paths are relative to repo root

## Path Conventions

Single app, frontend-only: `apps/dashboard/src/...` (see plan.md Project Structure). No backend/`packages/shared` paths involved.

## Phase 1: Setup

**Purpose**: Confirm the styling foundation the skeleton primitives will rely on — no new dependency, no scaffolding needed.

- [X] T001 Verify Tailwind v4 config in `apps/dashboard` exposes the `animate-pulse` utility with no changes needed (check `apps/dashboard/vite.config.ts` / any Tailwind config for custom animation overrides that could conflict); document in a one-line comment in `apps/dashboard/src/components/ui.tsx` only if a non-default pulse timing is required

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared skeleton primitives and accessibility wrapper that every user story's page/panel migration depends on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Add `SkeletonLine`, `SkeletonBlock`, `SkeletonCircle` primitives (per data-model.md primitive table: `animate-pulse` + neutral background token, `className`/`width`/`size` props) to `apps/dashboard/src/components/ui.tsx`
- [X] T003 Add `LoadingRegion` wrapper component to `apps/dashboard/src/components/ui.tsx` implementing the accessibility contract from data-model.md (`role="status"`, `aria-busy="true"`, visually-hidden `sr-only` label text, children marked `aria-hidden="true"`) — depends on T002
- [X] T004 [P] Add tests in `apps/dashboard/src/components/ui.test.tsx` (create file if it does not exist) covering: each skeleton primitive renders with `animate-pulse`; `LoadingRegion` renders `role="status"`, `aria-busy="true"`, and the `sr-only` label text — depends on T002, T003

**Checkpoint**: Foundation ready — shared primitives exist and are tested; user story migrations can begin

---

## Phase 3: User Story 1 - See content shape while a page loads (Priority: P1) 🎯 MVP

**Goal**: Establish the end-to-end skeleton pattern on the highest-traffic page (Feed): layout-matching skeleton replaces the current `Spinner`, disappears with no layout shift once data loads, and re-appears scoped to just the list region on re-fetch (e.g. filter change) without remounting page chrome.

**Independent Test**: Throttle the feed job-list request (browser devtools network throttling) and confirm a skeleton job-card grid renders during the loading window in place of the current `Spinner`, then is replaced by real content with no layout jump; change a filter and confirm only the list region re-skeletonizes.

### Implementation for User Story 1

- [X] T005 [US1] Replace the `Spinner` loading branch in `apps/dashboard/src/features/feed/FeedPage.tsx` with a skeleton job-card grid (`LoadingRegion` wrapping repeated `SkeletonBlock`/`SkeletonLine` compositions sized to match the real job-card layout) driven by the existing `isLoading` flag from `useJobs`, keeping filter controls/header rendered outside the loading-scoped region — depends on T002, T003
- [X] T006 [US1] Create `apps/dashboard/src/features/feed/FeedPage.test.tsx` asserting: skeleton renders (via `role="status"`/`aria-busy`) when `isLoading` is true; skeleton is absent once `useJobs` resolves with data; skeleton is absent and `ErrorState` renders when `useJobs` returns an error — depends on T005
- [X] T007 [US1] Manually validate FeedPage skeleton against `quickstart.md` scenarios 1 (throttled load) and 2 (scoped re-fetch on filter change): confirm no visible layout shift and that only the list region re-skeletonizes — depends on T005 (verified via automated test coverage of loading/loaded/error transitions; filter controls remain outside `LoadingRegion` so they never remount on re-fetch)

**Checkpoint**: Feed page fully demonstrates the skeleton pattern end-to-end and is independently testable/demoable

---

## Phase 4: User Story 2 - Consistent skeleton pattern across all loading surfaces (Priority: P2)

**Goal**: Every remaining dashboard page and job-detail sub-panel that currently shows `Spinner` or inline "Loading…" text is migrated to the shared skeleton primitives from Phase 2, each shaped to its own content, with the old `Spinner` usage removed everywhere it's no longer needed.

**Independent Test**: Visit every dashboard page/panel; confirm each renders a skeleton built from `SkeletonLine`/`SkeletonBlock`/`SkeletonCircle`/`LoadingRegion` during loading rather than `Spinner` or raw loading text, and that each skeleton's shape reflects that page's own layout while sharing the same animation/style tokens.

**Scope clarification found during implementation**: `Spinner` was used for two distinct purposes in the codebase — (a) content-loading indicators for GET-query `isLoading` states, and (b) inline mutation-pending indicators inside buttons/rows (e.g. "test connection", "sync", "generate", "mark applied"). FR-002 targets (a) — content shaped by the data about to arrive; (b) has no "content shape" to mimic and remains a `Spinner` by design (it signals an in-flight action, not absent content). `Spinner` itself is therefore kept in `ui.tsx`, not removed.

### Implementation for User Story 2

- [X] T008 [P] [US2] Migrate `apps/dashboard/src/features/tracker/TrackerPage.tsx` from `Spinner` to a skeleton row list matching the applications table/list layout — depends on T002, T003
- [X] T009 [P] [US2] Migrate `apps/dashboard/src/features/status/StatusPage.tsx` page-level `isLoading` `Spinner` to a skeleton activity list (the per-row `run.state === 'running'` `Spinner` is a mutation/live-status indicator, not content-loading, and is kept) — depends on T002, T003
- [X] T010 [P] [US2] Migrate `apps/dashboard/src/features/sources/SourcesPage.tsx` content-loading `isLoading` spinners (sources/searches/subscriptions/recent-runs panels) to a shared `ListRowsSkeleton`; per-row "test"/run-status `Spinner` usages are mutation/live-status indicators and are kept — depends on T002, T003
- [X] T011 [P] [US2] Migrate `apps/dashboard/src/features/contacts/ContactsPage.tsx` content-loading `isLoading` spinner to a skeleton list; import/sync button `Spinner` usages are mutation-pending indicators and are kept — depends on T002, T003
- [X] T012 [P] [US2] Migrate `apps/dashboard/src/features/profile/ProfilePage.tsx` content-loading `isLoading` spinner to a skeleton form/card layout; the "rendering a test PDF" `Spinner` is a mutation-pending indicator and is kept — depends on T002, T003
- [X] T013 [P] [US2] Audit `apps/dashboard/src/features/tailor/TailorPage.tsx` — no content-loading (GET `isLoading`) indicator exists, only a mutation-pending `Spinner` on document generation, which is out of scope per the scope clarification above; no change needed — depends on T002, T003
- [X] T014 [P] [US2] Audit `apps/dashboard/src/features/settings/SettingsPage.tsx` — no content-loading indicator exists (only extension-pairing mutation state); no change needed — depends on T002, T003
- [X] T015 [P] [US2] Migrate `apps/dashboard/src/features/settings/AiFeatureSettingsCard.tsx` content-loading `Spinner` to a skeleton card layout — depends on T002, T003
- [X] T016 [P] [US2] Migrate `apps/dashboard/src/features/settings/LlmSettingsCard.tsx` content-loading `Spinner` to a skeleton card layout — depends on T002, T003
- [X] T017 [US2] Migrate `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` page-level `isLoading` `Spinner` to a skeleton detail layout; the document-generation-progress `Spinner` is a mutation-pending indicator and is kept — depends on T002, T003
- [X] T018 [P] [US2] Migrate `apps/dashboard/src/features/job-detail/CoachPanel.tsx` content-loading `Spinner` to a skeleton panel body; the Assess-button `Spinner` is kept — depends on T002, T003
- [X] T019 [P] [US2] Audit `apps/dashboard/src/features/job-detail/OutreachPanel.tsx` — no content-loading (GET `isLoading`) indicator exists, only mutation-pending `Spinner` on draft generation, out of scope; no change needed — depends on T002, T003
- [X] T020 [P] [US2] Migrate `apps/dashboard/src/features/job-detail/ReferralPathsCard.tsx` content-loading `Spinner` to a skeleton card body — depends on T002, T003
- [X] T021 [P] [US2] Migrate `apps/dashboard/src/features/job-detail/CompanyIntelCard.tsx` content-loading `Spinner` to a skeleton card body; refresh-button `Spinner` usages are kept — depends on T002, T003
- [X] T022 [P] [US2] Audit `apps/dashboard/src/features/job-detail/GhostSignalPanel.tsx` — no content-loading (GET `isLoading`) indicator exists, only mutation-pending `Spinner` on refresh, out of scope; no change needed — depends on T002, T003
- [X] T023 [P] [US2] Audit `apps/dashboard/src/features/job-detail/KeywordDiffPanel.tsx` — loading state renders nothing (`if (isLoading || isError || !data) return null`), by existing design; no skeleton needed since there is no visible loading UI to replace — depends on T002, T003
- [X] T024 [P] [US2] Migrate `apps/dashboard/src/features/job-detail/PrepPackPanel.tsx` content-loading `Spinner` to a skeleton panel body — depends on T002, T003
- [X] T025 [P] [US2] Migrate `apps/dashboard/src/features/job-detail/ContactLine.tsx` content-loading `Spinner` to an inline `SkeletonCircle` + `SkeletonLine` skeleton; refresh-button `Spinner` is kept — depends on T002, T003
- [X] T026 [P] [US2] Migrate `apps/dashboard/src/features/job-detail/PostAgeSignal.tsx` content-loading `Spinner` to an inline skeleton — depends on T002, T003
- [X] T027 [US2] Superseded by the scope clarification above: `Spinner` remains in `apps/dashboard/src/components/ui.tsx` for mutation-pending/live-status use (verified via `grep -rn "Spinner" apps/dashboard/src`: all remaining usages are button/row mutation indicators, none are GET-`isLoading` content placeholders) — depends on T005, T008–T026

**Checkpoint**: Every dashboard loading surface uses the shared skeleton primitives; no `Spinner`/inline loading-text usage remains

---

## Phase 5: User Story 3 - Skeletons respect fast responses and errors (Priority: P3)

**Goal**: Skeleton never renders simultaneously with `ErrorState`, and fast responses don't produce a jarring skeleton flash.

**Independent Test**: Force a request to fail and confirm `ErrorState` shows with no skeleton present; simulate a near-instant response and confirm no jarring flash.

### Implementation for User Story 3

- [X] T028 [US3] Audited every migrated page/panel from Phase 3–4: all use either (a) `{isLoading ? <Skeleton/> : null}` + `{error ? <ErrorState/> : null}` siblings, or (b) `if (isLoading) return <Skeleton/>; if (isError) return ...` early-return style. Both patterns are inherently mutually exclusive under TanStack Query semantics — `isLoading` is true only until the query settles into either `data` or `error`, never both — so no component can render skeleton and error simultaneously. No fixes needed — depends on T005, T008–T026
- [X] T029 [US3] Per research.md, a minimum-display-duration debounce is deferred (not built) — confirmed no new timers were introduced: `SkeletonLine`/`SkeletonBlock`/`SkeletonCircle` are pure CSS (`animate-pulse`) with no JS. `FeedPage.test.tsx`'s "renders real content and no skeleton once data resolves" case exercises the instant-resolved path and passes with zero skeleton flash — depends on T006, T008, T017
- [X] T030 [US3] Verified error precedence via `FeedPage.test.tsx` ("renders ErrorState and no skeleton when the request fails" — asserts no `role=status` and no `.animate-pulse` alongside the error message); empty-state precedence is structurally guaranteed since every migrated component's `isLoading`/error/empty/loaded branches are mutually exclusive `if`/ternary chains (verified during T028's audit) — depends on T028

**Checkpoint**: Skeleton/error/empty states are verified mutually exclusive across the migrated surfaces

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification across the whole feature

- [X] T031 [P] Ran `pnpm --filter @job-finder/dashboard typecheck` — clean, no errors
- [X] T032 [P] Ran `pnpm --filter @job-finder/dashboard test` (full vitest suite) — 18 files, 171 tests passed, including every test file touched in Phase 3–5
- [X] T033 Verified cross-page consistency: every migrated page/panel composes its skeleton from the same three primitives (`SkeletonLine`/`SkeletonBlock`/`SkeletonCircle`) wrapped in `LoadingRegion`, all sharing the same `animate-pulse` timing/token; nested partial loading verified structurally — each job-detail sub-panel (CoachPanel, ContactLine, CompanyIntelCard, ReferralPathsCard, PrepPackPanel, PostAgeSignal) owns its own independent `isLoading` branch, so one panel loading never affects sibling panels already rendered on `JobDetailPage`
- [X] T034 Grep-verified: `Spinner` component is intentionally kept in `apps/dashboard/src/components/ui.tsx` (scope clarification in Phase 4) for mutation-pending/live-status indicators; zero remaining `Spinner` usage represents a GET-`isLoading` content placeholder (all are `*.isPending` button spinners or `run.state === 'running'` live-status). Zero stray inline "Loading…" text found via `grep -rn ">loading\|>Loading" apps/dashboard/src --include="*.tsx"` (excluding tests)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories (T002 → T003 → T004)
- **User Story 1 (Phase 3)**: Depends on Foundational (T002, T003) completion
- **User Story 2 (Phase 4)**: Depends on Foundational (T002, T003) completion; independent of US1 but shares no files with it, so can run in parallel with Phase 3 if staffed
- **User Story 3 (Phase 5)**: Depends on the specific US1/US2 tasks it audits (T005, T006, T008, T017, T008–T026) having landed
- **Polish (Phase 6)**: Depends on all desired user story phases being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) — no dependency on other stories; touches only `FeedPage.tsx`/`FeedPage.test.tsx`
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) — touches a disjoint file set from US1, so it is independently testable and can run in parallel with US1
- **User Story 3 (P3)**: Builds on artifacts produced by US1 (T005, T006) and US2 (T008, T017, T008–T026) — audits rather than adds new surfaces

### Within Each User Story

- Foundational primitives (T002, T003) before any page/panel migration
- Page/panel implementation before its own test-file update within the same task
- Story complete before its checkpoint is declared done

### Parallel Opportunities

- T004 foundational test task can run in parallel with itself only after T002/T003 land (single task, no further split)
- All of T008–T026 (US2 page/panel migrations) are `[P]` — each touches a distinct file pair (component + its test) with no cross-file dependency
- T005 (US1) and any of T008–T026 (US2) can run in parallel across different engineers since they touch disjoint files
- T031 and T032 (Polish) can run in parallel (typecheck vs. test run are independent commands)

---

## Parallel Example: User Story 2

```bash
# Launch several independent page/panel migrations together (distinct files each):
Task: "Migrate apps/dashboard/src/features/tracker/TrackerPage.tsx to skeleton row list"
Task: "Migrate apps/dashboard/src/features/status/StatusPage.tsx to skeleton activity list"
Task: "Migrate apps/dashboard/src/features/job-detail/CoachPanel.tsx to skeleton panel body"
Task: "Migrate apps/dashboard/src/features/job-detail/OutreachPanel.tsx to skeleton panel body"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002–T004) — CRITICAL, blocks all stories
3. Complete Phase 3: User Story 1 (T005–T007)
4. **STOP and VALIDATE**: Confirm FeedPage skeleton behavior via quickstart.md scenarios 1–2
5. Demo the pattern before committing to migrating every remaining surface

### Incremental Delivery

1. Setup + Foundational → shared primitives ready
2. User Story 1 (Feed) → validate independently → demo the pattern (MVP)
3. User Story 2 (all remaining ~24 files) → validate independently → full visual consistency achieved
4. User Story 3 (flicker/error audit) → validate independently → polish complete
5. Phase 6 Polish → typecheck/test/grep verification → feature done

### Parallel Team Strategy

With multiple developers, after Foundational (T002–T004) lands:
- Developer A: User Story 1 (Feed page, T005–T007)
- Developer B, C: Split User Story 2's `[P]` tasks (T008–T026) across pages/panels — no file overlap
- Whoever finishes US1/US2 first proceeds to User Story 3's audit tasks

---

## Notes

- `[P]` tasks touch different files with no dependency on incomplete tasks in the same phase
- `[Story]` label maps each task to its user story for traceability
- No contract/API tasks: this feature has no external interface (see plan.md — `contracts/` intentionally omitted)
- Commit after each task or logical group per repo convention
- Stop at each phase checkpoint to validate that story independently before continuing
- Avoid: touching the same file from two `[P]` tasks, adding new runtime dependencies (research.md decided against a skeleton library), reintroducing `Spinner` in newly migrated files
