# Phase 1 Data Model: Skeleton Loading States

This feature is purely presentational — no persisted entities, no DB/API schema changes.
The "model" here is the shape of the skeleton primitives and how loading state maps to
their rendering, derived from spec.md's Key Entities section.

## Skeleton primitive (component-level, not persisted)

Three composable primitives, all Tailwind-utility-only, added to
`apps/dashboard/src/components/ui.tsx`:

| Primitive | Purpose | Key props |
|---|---|---|
| `SkeletonLine` | Placeholder for a line of text (title, label, single field) | `width?: string` (Tailwind width class or inline style), `className?: string` |
| `SkeletonBlock` | Placeholder for a card/panel-shaped region (job card, panel body) | `className?: string` (caller controls width/height/rounding via Tailwind) |
| `SkeletonCircle` | Placeholder for avatar/icon/dot shapes | `size?: 'sm' \| 'md' \| 'lg'`, `className?: string` |

Shared visual contract (FR-005):
- All three use the same base treatment: `animate-pulse` + neutral background token
  consistent with existing `border-border-strong`/`bg-*` tokens already used in `ui.tsx`.
- No component owns layout arrangement (spacing, grid, count) — each feature page/panel
  composes its own arrangement of primitives (per research.md decision).

## Loading state → rendering mapping

Not a new data model — reuses each page/panel's existing TanStack Query
`isLoading`/`isPending` (and `isFetching` where a background re-fetch should also show a
scoped skeleton per FR-006) to decide between three mutually exclusive render branches per
region:

1. **Loading** (`isLoading`/`isPending` true, no previous data) → skeleton composition.
2. **Error** (`error` truthy) → existing `ErrorState` (unchanged).
3. **Loaded, empty** (data resolved, zero items) → existing `EmptyState` (unchanged).
4. **Loaded, populated** → real content.

State transitions are driven entirely by existing query hooks (`useJobs`,
`useApplications`, `useActivity`, etc.) — no new state, no new hook required beyond
optionally a shared `aria-busy` wrapper.

## Accessibility contract

Each skeleton-rendering region wraps its skeleton primitives in:
```
<div role="status" aria-busy="true" aria-label="Loading <thing>…">
  {/* visually-hidden text duplicate of aria-label for screen readers that ignore aria-label on div */}
  <span className="sr-only">Loading <thing>…</span>
  {/* skeleton primitives, aria-hidden="true" */}
</div>
```
This keeps parity with the existing `Spinner`'s `label` prop semantics while satisfying
FR-007.
