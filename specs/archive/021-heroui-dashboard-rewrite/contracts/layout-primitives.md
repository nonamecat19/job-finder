# Contract: Layout Primitives

**Location**: `apps/dashboard/src/components/layout/`
**Consumers**: all nine pages and every feature panel.

These are the only layout APIs page authors may use. A page that needs layout CSS beyond
these props is a signal the contract is wrong — fix the primitive, not the page (FR-018).

---

## `DashboardGrid`

```tsx
type DashboardGridProps = {
  /** Inherited from the route's layout mode; pages rarely pass this directly. */
  variant?: 'flow' | 'fit';   // default: 'flow'
  children: ReactNode;        // expected to be Tile elements
  className?: string;
};
```

**Guarantees**

| Aspect | Behaviour |
|---|---|
| Columns | 1 (<640) · 2 (640–1023) · 3 (1024–1919) · 4 (1920–2559) · 5 (≥2560) — FR-001 |
| Gap | `0.75rem` <640 · `1rem` 640–1023 · `1.25rem` ≥1024 — FR-003 |
| Max width | `140rem`, centred — FR-011 |
| Alignment | `items-start` in `flow`; `items-stretch` in `fit` |
| Rows | `fit` sets explicit row tracks so tiles divide the viewport height; `flow` uses implicit rows |

**Implementation notes**

- Custom breakpoints `3xl`/`4xl` come from the token contract; do not hardcode pixel media
  queries in the component.
- `fit` is conditional on `@media (min-width:1024px) and (min-height:45rem)` — below either
  threshold it behaves as `flow` (short-viewport edge case, FR-020).
- The grid must not set `overflow`; page-level scrolling is the shell's concern.

---

## `Tile`

Replaces both `GridCard` (spacing) and `Surface` (chrome) — one component, not two.

```tsx
type SpanRule = 'compact' | 'standard' | 'wide' | 'tall' | 'feature' | 'full';

type TileProps = {
  title?: ReactNode;
  action?: ReactNode;                 // header-right affordance
  footer?: ReactNode;
  span?: SpanRule;                    // default: 'standard'
  tone?: 'default' | 'inverse' | 'quiet';  // default: 'default'
  scroll?: boolean;                   // body scrolls internally
  scrollLabel?: string;               // required when scroll — labels the scroll region
  state?: 'ready' | 'loading' | 'empty' | 'error';  // default: 'ready'
  emptyMessage?: ReactNode;           // shown when state === 'empty'
  error?: unknown;                    // shown when state === 'error'
  onRetry?: () => void;               // retry affordance for the error state
  children: ReactNode;                // body, rendered only when state === 'ready'
  className?: string;
};
```

**Guarantees**

- **Grid footprint is state-independent.** `loading`, `empty`, and `error` occupy exactly the
  cells `span` allocates (FR-009, SC-007). Never conditionally render a `Tile` away — pass a
  state instead.
- **`min-w-0` and `min-h-0` are always applied**, so a tile can shrink inside its track. This
  is what stops `overflow-y-auto` children from blowing past the viewport in `fit` mode.
- **`scroll: true`** wraps the body in a focusable region — `tabIndex={0}`, `role="region"`,
  `aria-label={scrollLabel}` — with a visible overflow affordance (HeroUI `ScrollShadow`).
  Without `tabIndex`, a scrollable div with no focusable children is keyboard-unreachable in
  Firefox and Safari (FR-021).
- **Styling is closed.** `className` may adjust internal layout (flex direction, gap of the
  body) but must not set background, border, radius, padding, or colour — those come from the
  token contract (FR-003, T3).
- **`tone="inverse"`** is the reference design's high-contrast tile (dark tile on a light
  field, or the reverse). It uses `--foreground`/`--background` inversion, never the accent
  (T5).

**Span translation** — owned by the primitive, not the caller (see data-model E3):

| `span` | 5col | 4col | 3col | 2col | 1col |
|---|---|---|---|---|---|
| `compact` / `standard` | 1×1 | 1×1 | 1×1 | 1×1 | 1×1 |
| `wide` | 2×1 | 2×1 | 2×1 | 2×1 | 1×1 |
| `tall` | 1×2 | 1×2 | 1×2 | 1×2 | 1×1 |
| `feature` | 3×2 | 2×2 | 2×2 | 2×1 | 1×1 |
| `full` | 5×1 | 4×1 | 3×1 | 2×1 | 1×1 |

Row spans are enforced in `fit` mode and advisory (`min-height`) in `flow`.

---

## `TileStates`

Internal presentations used by `Tile`; exported for panels that render sub-regions.

```tsx
function TileSkeleton(props: { lines?: number; className?: string }): JSX.Element;
function TileEmpty(props: { children: ReactNode; icon?: ReactNode }): JSX.Element;
function TileError(props: { error: unknown; onRetry?: () => void }): JSX.Element;
```

`TileError` renders a human-readable message plus a retry control when `onRetry` is given,
and never rethrows — a failing widget stays contained to its own cell.

---

## `PageHeader`

```tsx
type PageHeaderProps = {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
};
```

Unchanged API, restyled. Constraint: total rendered height ≤ one grid row's worth of vertical
space (User Story 4, scenario 3) — the description truncates rather than wrapping to a third
line at desktop widths.

---

## Shell layout mode

```tsx
type LayoutMode = 'flow' | 'fit';
```

- Declared per route in `app/routes.tsx`; `fit` for `/`, `/status`, `/tracker`, `flow` for the
  rest (data-model E4). An undeclared route defaults to `flow`.
- The shell resolves the mode once and applies it to `<main>`: `fit` →
  `lg:h-[100dvh] lg:overflow-hidden` (gated on the min-height query); `flow` → normal document
  scroll.
- Tiles cannot override the mode (V13). A page needing a different mode changes its route
  declaration.

---

## Migration notes

| Removed | Replacement |
|---|---|
| `GridCard` | `Tile` (`span` prop carries over: `narrow`→`standard`, `wide`→`wide`, `full`→`full`) |
| `Surface` (from `ui.tsx`) | `Tile` chrome |
| `LoadingRegion`, `SkeletonLine/Block/Circle` | `Tile state="loading"` / `TileSkeleton` |
| `EmptyState`, `ErrorState` | `Tile state="empty" \| "error"` / `TileEmpty`, `TileError` |
| `Button`, `Input`, `Textarea`, `Select`, `Checkbox`, `Field`, `Chip`, `Spinner` | HeroUI subpath imports (`@heroui/react/button`, …) |
| `ScoreBadge`, `GhostBadge`, `HealthDot` | Keep as app-specific components, restyled onto tokens + HeroUI `Chip`/`Tag` |

Import HeroUI from subpaths, never the barrel, to keep the initial chunk within the bundle
budget (research R8).
