# Implementation Plan: HeroUI Tile-Grid Dashboard Rewrite

**Branch**: `021-heroui-dashboard-rewrite` | **Date**: 2026-07-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/021-heroui-dashboard-rewrite/spec.md`

## Summary

Rebuild `apps/dashboard` as a monochrome tile grid on HeroUI v3. Three things change and
nothing else does:

1. **Component foundation** — `@heroui/react@^3.2.2` replaces `components/ui.tsx` and the two
   live Radix imports; five unused Radix packages are deleted (see research R4).
2. **Token layer** — `index.css` is re-authored against HeroUI's CSS-variable contract with a
   greyscale scale, one chromatic accent, and subdued status colours (R2, R3).
3. **Layout** — a `DashboardGrid` / `Tile` pair replaces the current 3-column
   `DashboardGrid`+`GridCard`, adding 4- and 5-column breakpoints, multi-row spans, and the
   hybrid fit/flow scroll model (R5, R6).

No API route, query hook, or data shape is touched. Everything ships as one deliverable
(clarification Q2), verified by a Playwright viewport matrix (R7).

## Technical Context

**Language/Version**: TypeScript 5.6, React 19.0

**Primary Dependencies**: `@heroui/react` ^3.2.2 (new) · Tailwind CSS 4.0 · Vite 6 ·
TanStack Query 5 · react-router-dom 7 · retained: `@dnd-kit/*`, `@tanstack/react-virtual`,
`clsx`, `tailwind-merge`, `lucide-react` · removed: all six `@radix-ui/*` direct deps ·
added dev: `@axe-core/playwright`

**Storage**: N/A — presentation-layer change only

**Testing**: `vitest` + Testing Library (24 existing files) for behaviour and widget
loading/empty/error states; `@playwright/test` (existing config, 3 existing specs) for the
layout/overflow/contrast viewport matrix

**Target Platform**: Modern evergreen browsers; primary target a 2560×1440 desktop viewport

**Project Type**: Web application frontend (single app inside a pnpm monorepo)

**Performance Goals**: No runtime target set by the spec. Build gate only: post-rewrite
initial JS chunk (gzipped) ≤ pre-rewrite baseline + 100 KB, measured with
`pnpm --filter @job-finder/dashboard build` before and after (R8).

**Constraints**: Zero horizontal scroll from 320px to 3840px · WCAG AA contrast in both
appearances · no page-level scroll on overview pages at ≥1024px with ≥45rem height ·
virtualized feed list must keep virtualizing inside a bounded tile · no behavioural
regressions (all existing tests pass or change only for markup/selectors)

**Scale/Scope**: 9 routes · ~35 feature components · ~6,000 LOC of page/panel TSX ·
24 vitest files · 1 stylesheet · 1 shell

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Applies? | Assessment |
|---|---|---|
| I. No Auto-Apply, Ever | Yes | No submission path is added or altered. Primary-action styling must not make any apply/send control easier to trigger accidentally — buttons that trigger outbound actions keep their existing confirmation flows (`ConfirmDialog` is ported to HeroUI's `AlertDialog`, not dropped). **PASS** |
| II. Grounded Generation | No | No prompt, generation, or post-processing code is touched. **N/A** |
| III. Typed Contracts Across Boundaries | Yes | No Go↔TS boundary changes; no generated file is hand-edited; `packages/shared` is untouched. New types are local UI props only. **PASS** |
| IV. Test Discipline Per Language | Yes | Dashboard-only change → `vitest` is the required suite, extended with Playwright for layout. `make test-lint` still runs before merge since the change is large, even though it stays inside one app. **PASS** |
| V. Local-First, Self-Hosted | Yes | HeroUI is a build-time npm dependency with no runtime service calls. No fonts, CSS, or scripts may be loaded from a CDN — the `Inter` font and all assets stay self-hosted/bundled. **PASS with constraint** |

**Additional gate from Technology & Architecture Constraints**: the constitution names the
dashboard stack as "React + Vite + TanStack Query + dnd-kit + Tailwind". HeroUI is an
addition to that list, not a replacement of any named element — dnd-kit and Tailwind are
explicitly retained. No amendment required.

**Workflow gate**: the constitution requires design docs under `plan/` for non-trivial
features. This feature's design lives in `specs/021-heroui-dashboard-rewrite/` per the newer
Spec Kit convention (as did 001–020); treating `specs/` as the successor to `plan/` is the
established practice in this repo and is noted here rather than silently assumed.

**Result**: PASS — no violations, Complexity Tracking not required.

*Post-Phase-1 re-check*: PASS. The design adds no new projects, no new services, no data
layer, and no cross-app boundary. The one judgement call — replacing a working component
layer wholesale rather than incrementally — is a user decision recorded in clarification Q1,
not an architectural expansion.

## Project Structure

### Documentation (this feature)

```text
specs/021-heroui-dashboard-rewrite/
├── plan.md              # This file
├── spec.md              # Feature specification (clarified)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── theme-tokens.md      # CSS custom-property contract
│   └── layout-primitives.md # DashboardGrid / Tile / shell component API
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
apps/dashboard/
├── src/
│   ├── index.css                       # REWRITTEN — HeroUI import + monochrome tokens
│   ├── app/
│   │   ├── shell.tsx                   # REWRITTEN — mono chrome, fit/flow main modes
│   │   ├── routes.tsx                  # edited — declares each route's layout mode
│   │   └── providers.tsx               # unchanged (HeroUI needs no provider)
│   ├── components/
│   │   ├── layout/
│   │   │   ├── DashboardGrid.tsx       # REWRITTEN — 1/2/3/4/5 cols, fit|flow variant
│   │   │   ├── Tile.tsx                # NEW — replaces GridCard + Surface
│   │   │   ├── TileStates.tsx          # NEW — loading / empty / error presentations
│   │   │   ├── PageHeader.tsx          # restyled
│   │   │   └── GridCard.tsx            # DELETED
│   │   ├── ui.tsx                      # DELETED — superseded by HeroUI
│   │   ├── toast.tsx                   # REWRITTEN on HeroUI toast
│   │   └── VirtualList.tsx             # kept; bounded-height contract added
│   └── features/                       # all 9 pages + panels recomposed as tiles
└── tests/e2e/
    ├── layout.spec.ts                  # NEW — viewport matrix (SC-001/004/010)
    ├── contrast.spec.ts                # NEW — axe colour-contrast (SC-006)
    └── {feed,navigation,sources}.spec.ts  # existing — selectors updated as needed
```

**Structure Decision**: All work is confined to `apps/dashboard`. The existing
`src/app` / `src/components` / `src/features` / `src/lib` split is kept — this is a rewrite
of the presentation inside an established structure, not a restructure. `apps/api` and
`packages/shared` are not touched, so no tygo/sqlc regeneration is involved.

## Implementation Phases

Ordering is a dependency chain, not a shipping schedule — nothing merges to `master` until
the last phase is green (clarification Q2).

| Phase | Work | Gate |
|---|---|---|
| **A. Foundation** | Install HeroUI, delete unused Radix deps, rewrite `index.css` against the token contract, record bundle baseline | App builds; existing vitest suite passes |
| **B. Primitives** | `DashboardGrid`, `Tile`, `TileStates`, restyled `PageHeader`; delete `GridCard` | Primitive-level vitest tests pass |
| **C. Shell** | Sidebar/mobile nav on HeroUI, fit/flow `<main>` modes, route layout-mode wiring, `NotificationBell` | `navigation.spec.ts` passes |
| **D. Component swap** | `toast.tsx` → HeroUI toast, `ConfirmDialog` → `AlertDialog`, all `ui.tsx` consumers migrated, `ui.tsx` deleted, remaining Radix deps removed | Zero `@radix-ui` direct imports; full vitest suite passes |
| **E. Page conversion** | 9 pages recomposed as tiles with declared spans and states — flow pages first (job detail, profile, tailor, settings, sources, contacts), then fit pages (feed, status, tracker) | Per-page vitest state tests pass |
| **F. Verification** | `layout.spec.ts`, `contrast.spec.ts`, token-audit check, bundle delta, `make test-lint` | All SC-001…SC-010 asserted green |

Fit pages come last in Phase E because they depend on the shell's fit mode (Phase C) and are
the ones where the `min-h-0` grid-child pitfall (R6) bites.

## Risks

| Risk | Mitigation |
|---|---|
| `bg-overlay`'s meaning silently flips (22 call sites, R3) | Rewrite all 22 to `bg-surface-tertiary` in the same commit as the token swap; grep-gate that `bg-overlay` appears nowhere outside floating components |
| Fit-mode tiles push past the viewport because grid children default to `min-height:auto` | `min-h-0` is baked into the `Tile` primitive, not left to page authors; SC-010 asserts it |
| Feed virtualization breaks inside a bounded tile | `VirtualList` gets an explicit height contract; feed conversion keeps its existing vitest coverage and `feed.spec.ts` |
| One large unreviewable diff (consequence of Q1+Q2) | Phases A–F land as separate commits on the feature branch; only the merge is big-bang |
| HeroUI's dark-mode convention is inverted vs. the repo's `.light` class | Switch to `data-theme` in Phase A, before any page work depends on it |
| Bundle growth from barrel imports | Subpath imports only; delta measured in Phase F against the Phase A baseline |

## Complexity Tracking

> No Constitution Check violations. Section intentionally empty.
