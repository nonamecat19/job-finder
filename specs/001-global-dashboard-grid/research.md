# Research: Global Dashboard Grid Layout

**Feature**: Global Dashboard Grid Layout  
**Date**: 2026-07-28  
**Phase**: 0 — Design Decisions & Pattern Research

---

## Decision: CSS Grid via Tailwind Utility Classes Wrapped in React Components

**Decision**: Implement the grid using Tailwind CSS v4's native `grid` utilities (`grid-cols-3`, `col-span-*`, `gap-5`) wrapped in two React layout primitives: `DashboardGrid` (container) and `GridCard` (item with span control).

### Rationale

1. **Tailwind v4 native support**: The project already uses Tailwind v4 with `@theme inline`. CSS Grid is fully supported via `grid`, `grid-cols-{n}`, `col-span-{n}`, `gap-{n}`, and responsive prefixes (`md:`, `lg:`). No custom CSS or additional dependencies needed.

2. **CSS Grid over Flexbox**: Flexbox is 1-dimensional (row OR column). CSS Grid is 2-dimensional, which is essential for a bento-box layout where cards have both row AND column placement. Grid also handles variable-height cards more naturally with `align-items: start` (no forced equal-height rows).

3. **No masonry library needed**: The spec defines fixed column spans (1, 2, 3 columns), not the free-floating masonry pattern where cards slot into the shortest column. A masonry library (e.g., `react-masonry-css`) would add an unnecessary dependency and complexity for a fixed-span grid.

4. **React component wrapper**: A component API (`DashboardGrid`, `GridCard`) provides:
   - Semantic prop names (`span="wide"`, `span="narrow"`, `span="full"`) instead of raw `className` manipulation
   - Single point of control for gap values, breakpoints, and grid behavior
   - Future-proofing: if we later switch to a different grid implementation, only the component internals change

### Alternatives Considered

| Approach | Why Rejected |
|----------|--------------|
| CSS Grid with custom CSS classes | Violates Tailwind-first project convention; `index.css` uses `@theme inline` for all styling |
| CSS Flexbox with `flex-wrap` | Cannot achieve true 2D bento-box spanning; cards would flow left-to-right without column control |
| Masonry library (react-masonry-css) | Overkill for fixed-span cards; adds dependency; breaks semantic responsive control |
| CSS Grid `auto-fit` / `minmax()` | Too unpredictable for a dashboard where specific cards MUST be wide vs narrow; would require per-card media queries |

---

## Decision: Three Breakpoints with Semantic Span Degradation

**Decision**: Use the breakpoints already implied by the existing shell layout: `<768px` (1 col), `768px–1023px` (2 col), `≥1024px` (3 col). Column span rules degrade proportionally:

| Desktop Span | Tablet (2-col) | Mobile (1-col) |
|--------------|----------------|----------------|
| 1 column     | 1 column       | 1 column       |
| 2 columns    | 2 columns (full)| 1 column       |
| 3 columns    | 2 columns (full)| 1 column       |

### Rationale

- The existing `app/shell.tsx` already uses `md:` breakpoint at 768px for the sidebar switch. Aligning grid breakpoints with shell breakpoints avoids layout "jumps" at different widths.
- Semantic degradation is predictable: wide cards always become full-width on smaller screens, narrow cards stay narrow until they must stack.

---

## Decision: `align-items: start` for Independent Card Heights

**Decision**: The grid container uses `items-start` (Tailwind's `items-start`, equivalent to CSS `align-items: start`) so cards in the same row maintain their intrinsic height.

### Rationale

- The spec's edge cases explicitly call out: "Cards in the same row should have independent heights (no forced equal-height stretching) to prevent excessive whitespace in shorter cards."
- `items-start` is the default for CSS Grid when not explicitly set to `stretch`, but making it explicit in the component code documents the intent.

---

## Decision: Preserve Virtualized Lists As Single Grid Items

**Decision**: The feed page's `VirtualList` (TanStack Virtual) will be wrapped as a single `GridCard span="full"` rather than converting individual job cards to grid items.

### Rationale

- TanStack Virtual uses absolute positioning for row items. Converting these to CSS Grid items would break the virtualization logic (rows would no longer be absolutely positioned, and the virtualizer would lose height calculations).
- The spec's FR-010 explicitly allows this: "Virtualized lists MAY retain their vertical list layout internally if grid conversion would break virtualization."
- The feed page can still benefit from the grid by placing summary/filter cards alongside the list in a 2-column layout (list = wide, filters = narrow).

---

## Accessibility Considerations

- **DOM order**: CSS Grid does not change DOM order. Screen readers will read cards in source order, which is correct.
- **Keyboard navigation**: No change needed; cards remain `<section>` or `<article>` elements with standard tab order.
- **Reduced motion**: Grid reflow on resize is a layout change, not an animation. No `prefers-reduced-motion` consideration needed.
- **Zoom / high DPI**: CSS Grid is zoom-responsive by default; columns will reflow as viewport width changes.

---

## No Additional Dependencies Required

The implementation requires zero new npm packages. All grid functionality is provided by:
- Tailwind CSS v4 (already in project)
- React (already in project)
- clsx + tailwind-merge (already in project, used by `cn()` helper)
