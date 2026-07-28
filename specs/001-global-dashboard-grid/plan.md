# Implementation Plan: Global Dashboard Grid Layout

**Branch**: `001-global-dashboard-grid` | **Date**: 2026-07-28 | **Spec**: [specs/001-global-dashboard-grid/spec.md](specs/001-global-dashboard-grid/spec.md)

**Input**: Feature specification from `/specs/001-global-dashboard-grid/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

Refactor all 9 dashboard pages to use a consistent, responsive bento-box CSS Grid layout system. Introduce a reusable `DashboardGrid` container component and `GridCard` wrapper that establish 3-column (desktop) / 2-column (tablet) / 1-column (mobile) grids with semantic column spanning. Wide analytical panels span 2 columns; narrow utility cards occupy 1 column. The change is purely presentational — no data model or API changes — and integrates with the existing Tailwind v4 theme and `Surface` card primitive.

## Technical Context

**Language/Version**: TypeScript 5.x / React 18+ / Tailwind CSS v4

**Primary Dependencies**: React, Vite, Tailwind CSS v4 (with `@theme inline`), clsx + tailwind-merge (`cn()` helper)

**Storage**: N/A — purely presentational feature, no data persistence

**Testing**: vitest (dashboard unit tests), `make test-lint` (cross-app gate)

**Target Platform**: Web browsers (responsive: 320px–1920px+ viewports)

**Project Type**: Web application (dashboard frontend)

**Performance Goals**: Layout reflows at 60fps; grid recalculation on resize must not cause jank; no layout shift during initial paint

**Constraints**: Zero horizontal scroll at any viewport width; must coexist with existing sticky sidebar (`md:grid-cols-[16rem_1fr]` shell); all existing card content and interactivity preserved

**Scale/Scope**: 9 pages, ~30–40 card components to wrap in grid spans, 2 new layout primitives to create

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. No Auto-Apply | ✅ Pass | Pure UI layout change; no application logic touched |
| II. Grounded Generation | ✅ Pass | No LLM-generated content involved |
| III. Typed Contracts | ✅ Pass | No cross-language boundary changes; `apps/dashboard` only |
| IV. Test Discipline | ⚠️ Check | `vitest` for new components; `make test-lint` must pass before merge |
| V. Local-First | ✅ Pass | No external API dependencies introduced |

**Re-check after Phase 1**: All principles still pass. No violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/001-global-dashboard-grid/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (layout entities)
├── quickstart.md        # Phase 1 output (validation guide)
└── tasks.md             # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
apps/dashboard/src/
├── components/
│   ├── ui.tsx                 # Surface, Button, Chip (existing)
│   ├── layout/
│   │   ├── PageHeader.tsx     # Existing page header
│   │   ├── DashboardGrid.tsx  # NEW: grid container primitive
│   │   └── GridCard.tsx       # NEW: grid item wrapper with col-span props
│   └── ...
├── features/
│   ├── job-detail/
│   │   ├── JobDetailPage.tsx  # REFACTOR: wrap cards in DashboardGrid + GridCard
│   │   └── *.tsx              # REFACTOR: each panel gets grid span prop
│   ├── feed/
│   │   └── FeedPage.tsx       # REFACTOR: job cards in grid
│   ├── tracker/
│   │   └── TrackerPage.tsx    # REFACTOR: application cards in grid
│   ├── contacts/
│   │   └── ContactsPage.tsx   # REFACTOR: adopt grid
│   ├── sources/
│   │   └── SourcesPage.tsx    # REFACTOR: adopt grid
│   ├── profile/
│   │   └── ProfilePage.tsx    # REFACTOR: adopt grid
│   ├── settings/
│   │   └── SettingsPage.tsx   # REFACTOR: adopt grid
│   ├── status/
│   │   └── StatusPage.tsx     # REFACTOR: adopt grid
│   └── tailor/
│       └── TailorPage.tsx     # REFACTOR: adopt grid
├── app/
│   └── shell.tsx              # UNCHANGED: sidebar + main split preserved
└── index.css                  # UNCHANGED: Tailwind theme tokens
```

**Structure Decision**: Single-project web app structure. All changes confined to `apps/dashboard/src/`. Two new files in `components/layout/`, refactor of page-level components in `features/*/`. No backend or shared-package changes.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. All principles pass.

---

## Phase 0: Research — Complete

**Output**: [research.md](research.md)

### Key Decisions

| Topic | Decision | Rationale |
|-------|----------|-----------|
| Grid implementation | CSS Grid via Tailwind utilities wrapped in React components | Native Tailwind v4 support, 2D layout power, no new dependencies |
| Flexbox vs Grid vs Masonry | CSS Grid wins | Fixed-span bento layout needs 2D control; masonry overkill |
| Breakpoints | `<768px` 1-col / `768–1023px` 2-col / `≥1024px` 3-col | Aligns with existing shell `md:` breakpoint |
| Card height behavior | `items-start` (independent heights) | Prevents whitespace in short cards per spec edge cases |
| Virtualized lists | Kept as single grid item (`span="full"`) | TanStack Virtual absolute positioning incompatible with grid items |

**No [NEEDS CLARIFICATION] remain.**

---

## Phase 1: Design & Contracts — Complete

**Outputs**:
- [data-model.md](data-model.md) — Layout entities (GridContainer, GridCard, PageLayout) with span rules
- [quickstart.md](quickstart.md) — 5 validation scenarios + automated regression check
- **contracts/** — Skipped. This feature is purely internal UI with no external interfaces (no APIs, CLI, or public contracts).

### Constitution Re-Check Post-Design

All 5 principles still pass. No violations introduced.

| Principle | Status |
|-----------|--------|
| I. No Auto-Apply | ✅ Pass |
| II. Grounded Generation | ✅ Pass |
| III. Typed Contracts | ✅ Pass |
| IV. Test Discipline | ✅ Pass (vitest for `DashboardGrid`/`GridCard`; `make test-lint` gate) |
| V. Local-First | ✅ Pass |

---

## Next Step

Ready for **`/speckit-tasks`** to generate the dependency-ordered task list for implementation.

