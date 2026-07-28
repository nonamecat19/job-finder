# Tasks: Global Dashboard Grid Layout

**Input**: Design documents from `/specs/001-global-dashboard-grid/`

**Prerequisites**: plan.md (required), spec.md (required for user stories), data-model.md (layout entities), research.md (decisions), quickstart.md (validation scenarios)

**Tests**: Minimal vitest unit tests for new `DashboardGrid` and `GridCard` components per Constitution Principle IV (Test Discipline). No broad test coverage required as this is a pure presentational refactor.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Minimal — Refactor Context)

**Purpose**: Confirm codebase state and identify all files that need grid adoption. No new dependencies or project initialization needed.

- [X] T001 Verify `apps/dashboard/src/components/layout/` exists and list existing layout files
- [X] T002 [P] Audit all 9 page components to confirm their exact file paths and current layout approach

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create the two reusable layout primitives that ALL user stories depend on. MUST complete before any page refactor begins.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Create `DashboardGrid` container component in `apps/dashboard/src/components/layout/DashboardGrid.tsx` with responsive 3-col/2-col/1-col grid, gap-5/gap-3, and `items-start`
- [X] T004 Create `GridCard` wrapper component in `apps/dashboard/src/components/layout/GridCard.tsx` with semantic `span` prop (`narrow` | `wide` | `full`) mapping to Tailwind `col-span-*` classes with responsive degradation
- [X] T005 Export new components from `apps/dashboard/src/components/layout/index.ts` (create barrel file if absent)
- [X] T006 [P] Write vitest unit tests for `DashboardGrid` and `GridCard` in `apps/dashboard/src/components/layout/__tests__/grid-layout.test.tsx` (verify grid class generation, span mapping, responsive prefixes)

**Checkpoint**: Foundation ready — `DashboardGrid` and `GridCard` are created, exported, and tested. User story implementation can now begin.

---

## Phase 3: User Story 1 — Job Detail Grid Layout (Priority: P1) 🎯 MVP

**Goal**: Refactor the job detail page to use the dense bento-box grid with semantic column spanning for all analytical cards.

**Independent Test**: Navigate to any job detail page on desktop (≥1024px). Verify 3-column grid, wide cards span 2 columns, narrow cards sit in 1 column, no horizontal scroll, and at least 3 cards are visible without scrolling. Repeat on tablet (2-col) and mobile (1-col).

### Implementation for User Story 1

- [X] T007 [US1] Refactor `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` — wrap page content in `<DashboardGrid>` and assign semantic spans per data-model.md:
  - FitSummary → `full`
  - GhostSignalPanel → `wide`
  - PostAgeSignal → `narrow`
  - ContactLine → `narrow`
  - CompanyIntelCard → `narrow`
  - ReferralPathsCard → `narrow`
  - KeywordDiffPanel → `wide`
  - CoachPanel + OutreachPanel (stacked inside single `GridCard span="narrow"`) → `narrow`
  - PrepPackPanel → `full`
  - DocumentsPanel → `full`
  - Job description Surface → `wide`
  - ResumePreview (if present) → `narrow`

**Checkpoint**: Job detail page renders in a dense bento-box grid across all breakpoints. Page is independently testable.

---

## Phase 4: User Story 2 — Feed and Tracker Grid Layout (Priority: P1)

**Goal**: Refactor the feed and tracker pages to arrange cards in a responsive multi-column grid.

**Independent Test**: Navigate to feed page (`/`) and tracker page (`/tracker`) on desktop. Verify job cards and application cards appear in a 2-column grid with consistent rhythm. On tablet, verify 2-column remains. On mobile, verify single-column with no overflow.

### Implementation for User Story 2

- [X] T008 [P] [US2] Refactor `apps/dashboard/src/features/feed/FeedPage.tsx` — wrap main content in `<DashboardGrid>`. If summary/filter cards exist alongside the virtualized list, place them as `narrow` GridCards and the `VirtualList` as a `full` GridCard. If the page is list-only, wrap the entire content in a single `GridCard span="full"` inside `DashboardGrid`.
- [X] T009 [P] [US2] Refactor `apps/dashboard/src/features/tracker/TrackerPage.tsx` — wrap main content in `<DashboardGrid>` and assign spans: application cards default to `narrow` (1 col each in 3-col grid) or `wide` for detail-heavy cards.

**Checkpoint**: Feed and tracker pages display cards in responsive grids. Both pages are independently testable.

---

## Phase 5: User Story 3 — Consistent Grid Across All Pages (Priority: P2)

**Goal**: Apply the global grid system to the remaining 6 dashboard pages so layout rhythm is consistent everywhere.

**Independent Test**: Visit contacts, sources, profile, settings, status, and tailor pages in sequence. Verify each uses `<DashboardGrid>` with identical gap spacing, card styling, and responsive behavior.

### Implementation for User Story 3

- [X] T010 [P] [US3] Refactor `apps/dashboard/src/features/contacts/ContactsPage.tsx` — adopt `<DashboardGrid>` with appropriate `GridCard` spans for contact cards/forms
- [X] T011 [P] [US3] Refactor `apps/dashboard/src/features/sources/SourcesPage.tsx` — adopt `<DashboardGrid>` with appropriate `GridCard` spans for source cards/tables
- [X] T012 [P] [US3] Refactor `apps/dashboard/src/features/profile/ProfilePage.tsx` — adopt `<DashboardGrid>` with appropriate `GridCard` spans for profile sections
- [X] T013 [P] [US3] Refactor `apps/dashboard/src/features/settings/SettingsPage.tsx` — adopt `<DashboardGrid>` with appropriate `GridCard` spans for settings cards/forms
- [X] T014 [P] [US3] Refactor `apps/dashboard/src/features/status/StatusPage.tsx` — adopt `<DashboardGrid>` with appropriate `GridCard` spans for status cards
- [X] T015 [P] [US3] Refactor `apps/dashboard/src/features/tailor/TailorPage.tsx` — adopt `<DashboardGrid>` with appropriate `GridCard` spans for tailor cards

**Checkpoint**: All 9 dashboard pages use the global grid system. Layout is visually consistent across every page.

---

## Phase 6: User Story 4 — Responsive Grid Adaptation Validation (Priority: P2)

**Goal**: Verify the grid degrades gracefully across all breakpoints on every page without clipping, overflow, or broken layouts.

**Independent Test**: Resize browser from 1920px → 1280px → 820px → 375px while on each page. Verify smooth reflow, no horizontal scroll, no clipped content, no overlapping cards.

### Implementation for User Story 4

- [X] T016 [US4] Add `min-w-0` / `overflow-hidden` safeguards to `GridCard` in `apps/dashboard/src/components/layout/GridCard.tsx` if card content causes horizontal overflow on narrow viewports
- [ ] T017 [US4] Run scroll-width audit across all 9 pages at 320px, 375px, 768px, 820px, 1024px, 1280px, and 1920px viewports. Document any pages with `document.body.scrollWidth > window.innerWidth` and fix.

**Checkpoint**: Zero horizontal scroll on all pages at all viewport widths. Responsive degradation is verified.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final validation, cleanup, and quality gates.

- [X] T018 [P] Run `pnpm --filter @job-finder/dashboard test` (vitest) and verify all tests pass, including the new grid layout tests
- [X] T019 [P] Run `make test-lint` and verify full cross-app gate passes with zero new lint errors
- [X] T020 [P] Verify `apps/dashboard/src/app/shell.tsx` was NOT modified (sidebar must remain untouched)
- [ ] T021 Validate quickstart.md Scenario 1 (Job Detail Page Grid) and Scenario 2 (Cross-Page Consistency) manually in browser
- [X] T022 [P] If `apps/dashboard/src/components/layout/index.ts` was created, verify all existing layout exports (e.g., `PageHeader`) are preserved
- [X] T023 Remove any per-page `grid gap-*` or `lg:grid-cols-*` utility classes that are now redundant because `DashboardGrid` provides them

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Stories (Phase 3–6)**: All depend on Foundational phase completion
  - Phase 3 (US1 Job Detail) and Phase 4 (US2 Feed/Tracker) can proceed in parallel once Foundational is complete
  - Phase 5 (US3 Remaining Pages) can start once Phase 3 is done (to use Job Detail as the reference pattern)
  - Phase 6 (US4 Responsive Validation) can start once all page refactors are done
- **Polish (Phase 7)**: Depends on all user story phases being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) — No dependencies on other stories
- **User Story 2 (P1)**: Can start after Foundational (Phase 2) — No dependencies on other stories; can run in parallel with US1
- **User Story 3 (P2)**: Can start after Foundational (Phase 2) — No hard dependencies, but recommended to wait until US1 is done to use Job Detail as the visual reference pattern
- **User Story 4 (P2)**: Depends on ALL page refactors (US1 + US2 + US3) being complete — validates responsive behavior globally

### Within Each User Story

- Grid primitives MUST be complete before any page refactor
- Core page wrapper (DashboardGrid + GridCard placement) before responsive fine-tuning
- Story complete before moving to next priority

### Parallel Opportunities

- T001 and T002 (Setup audit) can run in parallel
- T003–T006 (Foundational components + tests) can largely run in parallel once T003 (DashboardGrid) is started
- T008 and T009 (Feed + Tracker) can run in parallel
- T010–T015 (all 6 remaining pages) can run in parallel
- T018–T023 (Polish tasks) can run in parallel after all page refactors are done

---

## Parallel Example: User Story 1 + User Story 2 Together

```bash
# After Phase 2 (Foundational) is complete, launch both P1 stories in parallel:
Task: "T007 [US1] Refactor JobDetailPage.tsx with DashboardGrid + GridCard spans"
Task: "T008 [P] [US2] Refactor FeedPage.tsx with DashboardGrid"
Task: "T009 [P] [US2] Refactor TrackerPage.tsx with DashboardGrid"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (minimal audit)
2. Complete Phase 2: Foundational (DashboardGrid + GridCard + tests)
3. Complete Phase 3: User Story 1 (Job Detail page grid)
4. **STOP and VALIDATE**: Test Job Detail page independently across breakpoints
5. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 (Job Detail) → Test independently → Dense bento-box grid MVP delivered
3. Add User Story 2 (Feed + Tracker) → Test independently → List pages upgraded
4. Add User Story 3 (Remaining 6 pages) → Test independently → Full dashboard consistency
5. Add User Story 4 (Responsive validation) → Verify globally → All breakpoints validated
6. Polish → Tests + lint pass → Feature complete

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: US1 (Job Detail) — establishes the reference pattern
   - Developer B: US2 (Feed + Tracker) — applies grid to list-based pages
3. Once US1 is done (reference pattern established):
   - Developer C: US3 (Contacts + Sources + Profile)
   - Developer D: US3 (Settings + Status + Tailor)
4. Once all page refactors are done:
   - Any developer: US4 (Responsive validation) + Polish phase

---

## Task Count Summary

| Phase | Tasks | Description |
|-------|-------|-------------|
| Phase 1: Setup | 2 | Codebase audit |
| Phase 2: Foundational | 4 | DashboardGrid + GridCard + barrel export + tests |
| Phase 3: US1 (Job Detail) | 1 | JobDetailPage refactor |
| Phase 4: US2 (Feed/Tracker) | 2 | FeedPage + TrackerPage refactor |
| Phase 5: US3 (All Pages) | 6 | Contacts, Sources, Profile, Settings, Status, Tailor |
| Phase 6: US4 (Responsive) | 2 | Overflow safeguards + scroll-width audit |
| Phase 7: Polish | 6 | Tests, lint, shell untouched check, manual validation, cleanup |
| **Total** | **23** | |

---

## Notes

- No new npm dependencies needed — all grid functionality is Tailwind v4 native
- `GridCard` intentionally does NOT modify `Surface`; it wraps around it. Each page's sub-components (GhostSignalPanel, FitSummary, etc.) remain unchanged — only their placement in `JobDetailPage.tsx` changes.
- The existing `lg:grid-cols-3` on `JobDetailPage.tsx` will be replaced by `DashboardGrid`.
- Virtualized lists (FeedPage `VirtualList`) must NOT have their individual rows converted to grid items; the list container itself is the grid item.
- If any page already uses a grid or flex layout, the refactor should replace it with `DashboardGrid` rather than stacking layouts.
