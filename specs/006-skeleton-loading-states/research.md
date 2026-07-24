# Phase 0 Research: Skeleton Loading States

No `NEEDS CLARIFICATION` markers remained in the Technical Context — this research
consolidates the decisions used to fill it in, based on the existing `apps/dashboard`
codebase.

## Decision: Build skeleton primitives with Tailwind CSS animation, no new dependency

**Rationale**: `apps/dashboard` already uses Tailwind 4 with `animate-spin` used by the
existing `Spinner` (`apps/dashboard/src/components/ui.tsx:62-69`). Tailwind ships
`animate-pulse` out of the box, which is the standard low-cost shimmer/pulse effect for
skeletons and requires zero new dependency, keeping `apps/dashboard/package.json`
unchanged.

**Alternatives considered**:
- A dedicated skeleton library (e.g. `react-loading-skeleton`): rejected — adds a
  dependency for a few lines of CSS the codebase can already produce, and would introduce
  a second animation system alongside existing Tailwind utility use.
- Custom CSS keyframe shimmer (gradient sweep) instead of `animate-pulse`: viable
  follow-up polish, but `animate-pulse` is sufficient for FR-005 (consistent timing) and
  matches the project's existing utility-class-only styling approach (no separate CSS
  files under `apps/dashboard/src`).

## Decision: Skeleton primitives added to existing `components/ui.tsx`, not a new module

**Rationale**: `Spinner`, `EmptyState`, `ErrorState`, `Surface`, `Chip`, etc. are all
defined in the single `apps/dashboard/src/components/ui.tsx` file (no
`components/ui/` directory exists in this repo, unlike some Shadcn-style layouts).
Following existing project convention (Constitution III implies consistency of shared
primitives) means adding `SkeletonLine`, `SkeletonBlock`, `SkeletonCircle` (or similar) to
that same file rather than creating a new directory structure.

**Alternatives considered**: New `components/skeletons/` directory per feature — rejected,
inconsistent with the current flat `ui.tsx` convention and adds import-path churn across
~25 files for no benefit at this scale.

## Decision: Each feature page composes its own skeleton layout from shared primitives

**Rationale**: FR-003 requires skeleton shape to approximate real content per page. A
single generic `<PageSkeleton />` cannot fit both a card-grid feed and a detail-panel
layout. Shared primitives (line/block/circle) give each page/panel author the building
blocks; each `*Page.tsx`/`*Panel.tsx` assembles its own skeleton composition (e.g.
`FeedPage` renders N skeleton job-cards, `CoachPanel` renders skeleton text lines).

**Alternatives considered**: One generic list-skeleton and one generic card-skeleton
covering all pages — rejected, job-detail sub-panels (coach, outreach, referral paths,
company intel, ghost signal, keyword diff, prep pack) have visually distinct shapes; a
one-size-fits-all component would either look wrong or need enough props to become as
complex as bespoke composition anyway.

## Decision: Accessibility via `role="status"` + `aria-busy`, reusing existing pattern

**Rationale**: `Spinner` currently has no explicit ARIA wiring beyond visual spin;
FR-007 requires skeletons not be read as real content and still convey busy state. The
standard, dependency-free approach is wrapping the loading region in
`aria-busy="true"` with a visually-hidden `role="status"` label (e.g. "Loading jobs…"),
consistent with the existing `Spinner`'s `label` prop pattern.

**Alternatives considered**: `aria-live="polite"` announcing on every skeleton mount —
rejected as noisier than needed; a single `role="status"` per loading region read once is
sufficient and matches Assumptions in spec.md (reuse standard patterns, no new a11y
framework).

## Decision: No minimum-display-duration debounce in initial rollout

**Rationale**: Spec.md Assumptions explicitly marks User Story 3 (anti-flicker duration)
as a nice-to-have, not a blocker. TanStack Query's default `isLoading`/`isPending` flags
are used as-is; a `useDeferredLoading`-style debounce hook can be added later without
touching the skeleton primitives themselves (kept as future work, not part of this plan's
scope).

**Alternatives considered**: Building a shared debounce hook now — deferred; would add
scope beyond FR-001–FR-007 for a P3 polish item.
