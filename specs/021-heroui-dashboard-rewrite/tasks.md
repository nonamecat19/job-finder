---

description: "Task list for HeroUI Tile-Grid Dashboard Rewrite"
---

# Tasks: HeroUI Tile-Grid Dashboard Rewrite

**Input**: Design documents from `/specs/021-heroui-dashboard-rewrite/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/)

**Tests**: Test tasks ARE included. The spec makes automated verification a success criterion
(SC-001, SC-004, SC-006, SC-007, SC-010) and clarification Q3 fixed the strategy, so tests are
required deliverables, not optional extras.

**Organization**: Grouped by user story. Note the honest dependency shape — this is a rewrite,
not additive work: US3 (pages) cannot be built without US1's `Tile` primitive, and US2's tokens
must land with US1 or the app renders two palettes. US1+US2 together form the MVP foundation.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US5, mapping to spec.md user stories
- All paths are repo-relative from `/home/nnc/Projects/job-finder`

## Path Conventions

Single frontend app: `apps/dashboard/src/`, tests in `apps/dashboard/src/**/*.test.tsx`
(vitest) and `apps/dashboard/tests/e2e/` (Playwright).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Baseline capture and dependency changes. Nothing renders differently yet.

- [X] T001 Record pre-rewrite baselines: run `pnpm --filter @job-finder/dashboard build` and save the gzipped initial-chunk size, plus `pnpm --filter @job-finder/dashboard test` pass count, into `specs/021-heroui-dashboard-rewrite/baseline.md` (research R8 budget is meaningless without this captured first)
- [X] T002 Add `@heroui/react@^3.2.2` to dependencies in `apps/dashboard/package.json` and run `pnpm install`
- [X] T003 [P] Add `@axe-core/playwright` to devDependencies in `apps/dashboard/package.json` (SC-006 tooling)
- [X] T004 [P] Remove the five unused Radix packages (`@radix-ui/react-select`, `react-switch`, `react-tabs`, `react-tooltip`, `react-slot`) from `apps/dashboard/package.json` — verified zero imports in research R4, so no code change accompanies this
- [X] T005 [P] Self-host the Inter font: remove the `fonts.googleapis.com` preconnect and stylesheet `<link>` from `apps/dashboard/index.html`, add the woff2 files under `apps/dashboard/public/fonts/`, and declare `@font-face` in `apps/dashboard/src/index.css` (Constitution V — no third-party runtime calls)
- [X] T006 [P] Verify `@heroui/styles` CSS resolves under vitest by adding a smoke render of one HeroUI component in `apps/dashboard/src/components/heroui-smoke.test.tsx`; adjust `apps/dashboard/vite.config.ts` test CSS handling only if it fails

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Stylesheet plumbing, theme convention, and the two live Radix replacements. Every
story below depends on this phase.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T007 Replace `@import 'tailwindcss'` with `@import '@heroui/styles'` in `apps/dashboard/src/index.css`, keeping the existing `:root` variable block temporarily so the app still builds and renders with old colours
- [X] T008 Add `--breakpoint-3xl: 120rem`, `--breakpoint-4xl: 160rem`, and `--container-dashboard: 140rem` to the `@theme` block in `apps/dashboard/src/index.css` (contracts/theme-tokens.md)
- [X] T009 Switch the appearance convention from the `.light` class to `data-theme`: set `data-theme="dark"` on `<html>` in `apps/dashboard/index.html`, rename the `:root.light` block to `[data-theme='light']`, and make the dark block `:root, [data-theme='dark']` (research R2 — HeroUI's convention is inverted relative to the current one). Note: no code applies `.light` today, so the light appearance is currently dead; this makes it reachable
- [X] T010 Replace the Radix toast implementation in `apps/dashboard/src/components/toast.tsx` with HeroUI's toast, preserving the `toastBus` API in `apps/dashboard/src/lib/toastBus.ts` so no caller changes; update `apps/dashboard/src/components/toast.test.tsx`
- [X] T011 Replace Radix Dialog with HeroUI `AlertDialog` in `apps/dashboard/src/features/profile/components/ConfirmDialog.tsx`, keeping the confirm-before-destructive-action behaviour intact (Constitution I)
- [X] T012 Remove `@radix-ui/react-toast` and `@radix-ui/react-dialog` from `apps/dashboard/package.json` and assert `grep -rn "@radix-ui" apps/dashboard/src` returns nothing
- [X] T013 [P] Add the `LayoutMode` type (`'flow' | 'fit'`) and the per-route mode map in `apps/dashboard/src/app/routes.tsx`, defaulting unlisted routes to `flow` (data-model E4, V12) — consumed by US1's grid variant and US4's shell

**Checkpoint**: App builds and behaves as before, on HeroUI's stylesheet, with zero direct Radix imports.

---

## Phase 3: User Story 1 - Ultra-Wide Tile Grid (Priority: P1) 🎯 MVP

**Goal**: A grid that reaches 4 columns at 1920px and 5 at 2560px, with multi-column/row tile spans.

**Independent Test**: Load any page at 2560px and count occupied grid columns — 5, aligned to shared tracks, no horizontal scroll.

### Tests for User Story 1

- [X] T014 [P] [US1] Unit tests for span-rule translation and grid class output in `apps/dashboard/src/components/layout/__tests__/grid-layout.test.tsx` (rewrite the existing file): assert each `SpanRule` maps to the per-breakpoint columns in data-model E3 and that no span exceeds the column count (V9)
- [ ] T015 [P] [US1] Create `apps/dashboard/tests/e2e/layout.spec.ts` asserting computed `grid-template-columns` track counts of 1/2/3/4/5 at 375/768/1280/1920/2560 across all nine routes (SC-001)

### Implementation for User Story 1

- [X] T016 [US1] Rewrite `apps/dashboard/src/components/layout/DashboardGrid.tsx` per contracts/layout-primitives.md: 1/2/3/4/5 columns via `sm:`/`lg:`/`3xl:`/`4xl:`, gap 0.75/1/1.25rem, `max-width: var(--container-dashboard)` centred, `variant?: 'flow' | 'fit'` (FR-001, FR-003, FR-011)
- [X] T017 [US1] Create `apps/dashboard/src/components/layout/Tile.tsx` implementing the full `TileProps` contract — `title`/`action`/`footer` chrome, `span`, `tone`, `scroll` + `scrollLabel`, always-applied `min-w-0 min-h-0` (research R6 — the single most likely layout bug), styling closed to the token set (FR-002, FR-003)
- [X] T018 [P] [US1] Create `apps/dashboard/src/components/layout/TileStates.tsx` exporting `TileSkeleton`, `TileEmpty`, `TileError` on HeroUI `Skeleton`/`EmptyState`, with a retry affordance that never rethrows (FR-009)
- [X] T019 [US1] Wire the `state` prop in `Tile.tsx` to `TileStates` so `loading`/`empty`/`error` render inside the same grid footprint as `ready` (V5, SC-007)
- [X] T020 [US1] Add internal-scroll support to `Tile.tsx` using HeroUI `ScrollShadow` with `tabIndex={0}`, `role="region"`, `aria-label={scrollLabel}` (FR-021, V6)
- [X] T021 [US1] Delete `apps/dashboard/src/components/layout/GridCard.tsx` and update `apps/dashboard/src/components/layout/index.ts` to export `DashboardGrid`, `Tile`, `TileStates`, `PageHeader`

**Checkpoint**: Primitives exist and are unit-tested. Pages still use the old ones — conversion is US3.

---

## Phase 4: User Story 2 - Monochrome Visual System (Priority: P1)

**Goal**: One greyscale token set with a single chromatic accent, applied everywhere.

**Independent Test**: Audit every page — all surfaces/borders/text resolve to neutral tokens; the accent appears only in the permitted roles.

### Tests for User Story 2

- [X] T022 [P] [US2] Create `apps/dashboard/src/index.css.test.ts` asserting token invariants T1–T4: every neutral is `oklch(L 0 0)`, exactly one non-status chromatic token, both `[data-theme]` blocks define identical token sets (SC-005)
- [X] T023 [P] [US2] Extend the same test file with the T3/T5 grep gate: no hex/`rgb()`/`oklch()` literal and no Tailwind palette utility (`bg-zinc-*`, `text-slate-*`, …) anywhere in `apps/dashboard/src/**/*.tsx`
- [ ] T024 [P] [US2] Create `apps/dashboard/tests/e2e/contrast.spec.ts` running axe's `color-contrast` rule over all nine routes in both `data-theme` values (SC-006)

### Implementation for User Story 2

- [X] T025 [US2] Author the dark and light token blocks in `apps/dashboard/src/index.css` per contracts/theme-tokens.md: greyscale neutrals at chroma 0, one accent hue, subdued `success`/`warning`/`danger`, project tokens `--faint`, `--border-strong`, `--accent-soft`, `--*-soft`; delete `--bg`, `--fg`, `--elevated`, `--primary*`, and the second hue `--accent: #22d3ee` (FR-005, FR-007, FR-008)
- [X] T026 [US2] Remove the decorative `radial-gradient` body background and `::selection` primary wash from `apps/dashboard/src/index.css`; raise `--radius` to the tile rounding chosen for the reference look (FR-006)
- [X] T027 [US2] Rewrite all 22 `bg-overlay` occurrences to `bg-surface-tertiary` across `apps/dashboard/src/` — **must land in the same commit as T025**, since `bg-overlay` still compiles afterwards but silently resolves to HeroUI's floating-surface colour (research R3)
- [X] T028 [P] [US2] Migrate `text-fg` → `text-foreground` (55 sites) and `bg-elevated` → `bg-surface-secondary` (23 sites) across `apps/dashboard/src/`
- [X] T029 [P] [US2] Migrate `*-primary` → `*-accent` (~21 sites) and `*-primary-soft` → `*-accent-soft` (~8 sites) across `apps/dashboard/src/`
- [X] T030 [US2] Remove the brand gradients (`from-primary`, `to-accent`, `bg-clip-text`) from `apps/dashboard/src/app/shell.tsx`'s `Brand`, replacing them with neutral treatment (FR-006, T5)

**Checkpoint**: The app is monochrome end-to-end and the token audit passes, even though pages are still laid out the old way.

---

## Phase 5: User Story 3 - Every Page Rebuilt as Widgets (Priority: P1)

**Goal**: All nine pages composed of `Tile` widgets with declared spans and states; `ui.tsx` gone.

**Independent Test**: Visit each page — every content block sits in a `Tile`, no bespoke card styling, and each widget shows a skeleton/empty/error state instead of collapsing.

**Depends on**: US1 (Tile primitive) and US2 (tokens). This is a rewrite; the dependency is real, not incidental.

### Tests for User Story 3

- [ ] T031 [P] [US3] Add per-widget state tests (loading / empty / error keep the grid footprint) to the existing job-detail panel test files under `apps/dashboard/src/features/job-detail/*.test.tsx` (SC-007, V5)
- [ ] T032 [P] [US3] Add the same state coverage for the feed, status, and settings widgets in `apps/dashboard/src/features/{feed,status,settings}/*.test.tsx`

### Implementation for User Story 3

- [X] T033 [US3] Replace the `ui.tsx` control set with HeroUI subpath imports across the codebase — `Button`, `Input`, `Textarea`, `Select`, `Checkbox`, `Field`, `Chip`, `Spinner` → `@heroui/react/button` etc. (never the barrel import, research R8). 39 files import from `components/ui`; this task is the mechanical import/prop migration
- [X] T034 [US3] Port the app-specific components `ScoreBadge`, `GhostBadge`, `HealthDot` out of `apps/dashboard/src/components/ui.tsx` into `apps/dashboard/src/components/badges.tsx`, restyled onto tokens + HeroUI `Chip`
- [ ] T035 [US3] Delete `apps/dashboard/src/components/ui.tsx` and `apps/dashboard/src/components/ui.test.tsx`; confirm no remaining imports of `components/ui`
- [X] T036 [P] [US3] Convert `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` and its nine panels to `Tile`s with declared spans (description = `feature`, signals = `wide`, contacts/intel/coach/outreach = `standard`)
- [X] T037 [P] [US3] Convert `apps/dashboard/src/features/profile/ProfilePage.tsx`, `IdentityForm`, `SectionList`, `SectionEditor`, and the entry forms under `components/entries/` to tiles — keep dnd-kit reordering working inside the tile
- [X] T038 [P] [US3] Convert `apps/dashboard/src/features/sources/SourcesPage.tsx` (507 LOC — the largest page) plus `roster/RosterPanel.tsx` and `roster/CandidatesPanel.tsx` to tiles
- [X] T039 [P] [US3] Convert `apps/dashboard/src/features/settings/SettingsPage.tsx`, `LlmSettingsCard.tsx`, `AiFeatureSettingsCard.tsx` to tiles
- [X] T040 [P] [US3] Convert `apps/dashboard/src/features/tailor/TailorPage.tsx` and `apps/dashboard/src/features/contacts/ContactsPage.tsx` to tiles
- [X] T041 [P] [US3] Convert `apps/dashboard/src/features/feed/FeedPage.tsx` to tiles, keeping `VirtualList` virtualizing inside a bounded-height tile (FR-017)
- [X] T042 [P] [US3] Convert `apps/dashboard/src/features/tracker/TrackerPage.tsx` to tiles with KPI `compact` tiles for headline metrics (User Story 3 scenario 2)
- [X] T043 [P] [US3] Convert `apps/dashboard/src/features/status/StatusPage.tsx` to tiles with `compact` KPI treatment for the queue/health metrics
- [X] T044 [US3] Update `apps/dashboard/src/app/RequireProfileConfig.tsx` and `apps/dashboard/src/features/notifications/NotificationBell.tsx` to the new primitives and tokens
- [X] T045 [US3] Fix selector-only breakages in the existing vitest suite and in `apps/dashboard/tests/e2e/{feed,sources}.spec.ts`; any *behavioural* assertion change is a regression to investigate, not to update (FR-016)

**Checkpoint**: All nine pages are tile grids in `flow` mode. Full vitest suite green.

---

## Phase 6: User Story 4 - Restyled Shell and Fit-Mode Layout (Priority: P2)

**Goal**: Monochrome shell chrome plus the hybrid scroll model — overview pages fill the viewport with internally scrolling tiles.

**Independent Test**: Load `/`, `/status`, `/tracker` at 2560×1440 — no page-level scroll, long lists scroll inside their tiles, every scroll region reachable by Tab.

### Tests for User Story 4

- [ ] T046 [P] [US4] Extend `apps/dashboard/tests/e2e/layout.spec.ts` with the fit-mode assertion: `scrollHeight <= clientHeight` on `/`, `/status`, `/tracker` at 1920 and 2560 (SC-010)
- [ ] T047 [P] [US4] Add keyboard-reachability assertions to `apps/dashboard/tests/e2e/contrast.spec.ts` (or a new `a11y.spec.ts`): every nav destination and primary control is tabbable with a visible focus ring (SC-009)

### Implementation for User Story 4

- [X] T048 [US4] Restyle `apps/dashboard/src/app/shell.tsx`: neutral sidebar surfaces, accent reserved for the active nav item, sticky desktop rail preserved, exactly one accessible element per nav destination retained (FR-012, US4 scenarios 1–2)
- [X] T049 [US4] Add fit/flow `<main>` modes to `apps/dashboard/src/app/shell.tsx`, reading the route's `LayoutMode` from T013: `fit` → `lg:h-[100dvh] lg:overflow-hidden` gated on `@media (min-width:1024px) and (min-height:45rem)`; `flow` → normal document scroll (FR-020)
- [X] T050 [US4] Implement fit-mode row tracks and `items-stretch` in `apps/dashboard/src/components/layout/DashboardGrid.tsx` so tiles divide the viewport height (contracts/layout-primitives.md)
- [X] T051 [US4] Switch `/`, `/status`, `/tracker` to `fit` mode and set `scroll` + `scrollLabel` on the tiles whose content can exceed their height (feed results, tracker list, status queues)
- [X] T052 [US4] Restyle `apps/dashboard/src/components/layout/PageHeader.tsx` to fit within one grid row's vertical space, truncating the description rather than wrapping to a third line (US4 scenario 3)

**Checkpoint**: Overview pages match the reference look; content-heavy pages still scroll normally.

---

## Phase 7: User Story 5 - Responsive Down-Scaling (Priority: P2)

**Goal**: The same tiles reflow to 3/2/1 columns with nothing clipped, overlapping, or overflowing.

**Independent Test**: Sweep 320px→3840px on every page — column counts change only at defined breakpoints, zero horizontal scroll.

### Tests for User Story 5

- [ ] T053 [P] [US5] Extend `apps/dashboard/tests/e2e/layout.spec.ts` with the horizontal-overflow assertion (`documentElement.scrollWidth <= clientWidth`) at every matrix width plus a 320px minimum-width check (SC-004, FR-010)
- [ ] T054 [P] [US5] Add the ultra-wide case at 3840px: column count stays capped at 5 and the grid stays centred within `--container-dashboard` (FR-011, Edge Cases)

### Implementation for User Story 5

- [X] T055 [US5] Verify and fix span degradation at each breakpoint across all nine pages — `feature` and `full` tiles must never exceed the available column count (FR-002, V9)
- [X] T056 [US5] Implement the short-viewport fallback: `fit` pages revert to `flow` below 45rem height so tiles never drop under a minimum readable height (Edge Cases, FR-020)
- [X] T057 [US5] Audit dense text tiles (job description, tailored resume output) for readable line length when spanning multiple columns — cap prose measure inside the tile (Edge Cases)
- [X] T058 [US5] Verify mobile (<640px): single column, safe edge padding, all nav reachable, no tile height constraints (US5 scenario 1, FR-020)

**Checkpoint**: All five stories complete; full viewport matrix green.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [X] T059 Run `pnpm --filter @job-finder/dashboard build` and compare the gzipped initial chunk against the T001 baseline; report the delta and confirm it is within baseline + 100 KB (research R8)
- [X] T060 [P] Verify no HeroUI barrel imports remain: `grep -rn "from '@heroui/react'" apps/dashboard/src` should return nothing — all imports use subpaths
- [X] T061 [P] Confirm the transitive-only Radix note holds: `@radix-ui/react-avatar` may appear in the lockfile via HeroUI, but zero `@radix-ui` entries remain in `apps/dashboard/package.json`
- [ ] T062 Run the full `quickstart.md` validation Steps 1–7, including the manual visual pass at 2560×1440 in both appearances
- [ ] T063 Run `make test-lint` (required by Constitution IV for a change of this size)
- [ ] T064 [P] Update `apps/dashboard/README.md` (or the repo README's dashboard section) to document the tile grid, span rules, layout modes, and the token contract for future page authors (FR-018)
- [X] T065 Remove `apps/dashboard/src/components/heroui-smoke.test.tsx` (T006 scaffolding) and any leftover legacy token aliases from `apps/dashboard/src/index.css`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
- **US1 (Phase 3)** and **US2 (Phase 4)**: Both depend only on Foundational; can run in parallel by different people (US1 touches `components/layout/`, US2 touches `index.css` + utility renames)
- **US3 (Phase 5)**: Depends on US1 **and** US2 — pages need the `Tile` primitive and the token set
- **US4 (Phase 6)**: Depends on US3 (pages must be tiles before fit mode is meaningful) and on T013
- **US5 (Phase 7)**: Depends on US3; T056 additionally depends on US4
- **Polish (Phase 8)**: Depends on everything

### Story Dependency Graph

```text
Setup → Foundational ─┬─→ US1 (grid primitives) ─┐
                      └─→ US2 (tokens)  ─────────┴─→ US3 (pages) ─┬─→ US4 (shell + fit)
                                                                   └─→ US5 (responsive) → Polish
```

US1 and US2 are the only genuinely parallel pair. The spec's stories are not independently
shippable here because the feature is a rewrite of one surface, not a set of additive
capabilities — the deliverable is big-bang by decision (clarification Q2).

### Parallel Opportunities

- Phase 1: T003, T004, T005, T006 in parallel
- Phase 3 vs Phase 4: two developers, no file overlap
- Phase 5: T036–T043 are eight independent page conversions, one per feature directory
- Phase 6: T046, T047 (tests) in parallel with each other
- Phase 8: T060, T061, T064 in parallel

---

## Parallel Example: User Story 3 page conversions

```bash
# Eight independent page conversions, no shared files:
Task: "Convert JobDetailPage + panels to tiles"          # T036
Task: "Convert ProfilePage + entry forms to tiles"       # T037
Task: "Convert SourcesPage + roster panels to tiles"     # T038
Task: "Convert SettingsPage + settings cards to tiles"   # T039
Task: "Convert TailorPage and ContactsPage to tiles"     # T040
Task: "Convert FeedPage to tiles (keep virtualization)"  # T041
Task: "Convert TrackerPage to tiles with KPI widgets"    # T042
Task: "Convert StatusPage to tiles with KPI widgets"     # T043
```

Run T033–T035 (the `ui.tsx` swap and deletion) **before** these — they all depend on the
control components existing in their new form.

---

## Implementation Strategy

### MVP scope

The smallest demonstrable increment is **Setup + Foundational + US1 + US2**: HeroUI installed,
monochrome tokens live, and the 5-column grid primitive built and unit-tested. That is not a
shippable product — pages still use the old layout — but it is the point where the design
direction is visible and reversible cheaply. Get sign-off on the palette and tile chrome here,
before spending Phase 5 on nine page conversions.

### Delivery sequence

1. Phases 1–2 → app runs on HeroUI, behaviour unchanged
2. Phases 3–4 (parallel) → primitives + palette, **review checkpoint**
3. Phase 5 → all nine pages converted (the bulk of the work)
4. Phase 6 → shell and fit mode
5. Phase 7 → responsive hardening
6. Phase 8 → budgets, gates, docs → merge

Commit per task or logical group; the branch merges to `master` only after T063 passes.

### Highest-risk tasks

- **T027** (`bg-overlay` rewrite) — fails silently if missed; must ship with T025
- **T017** (`min-h-0` in `Tile`) — omission breaks every fit-mode page in Phase 6
- **T041** (feed virtualization in a bounded tile) — the one conversion that can break behaviour rather than just looks

---

## Notes

- [P] tasks touch different files with no incomplete dependencies
- Tests here are required deliverables (SC-001/004/006/007/010), unlike the template default
- `apps/api` and `packages/shared` are not touched by any task — no sqlc/tygo regeneration
- Constitution I is live in T011: the confirm-before-destructive-action dialog is ported, never dropped
