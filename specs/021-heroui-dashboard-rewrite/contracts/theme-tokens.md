# Contract: Theme Tokens

**Consumers**: every component in `apps/dashboard`, plus HeroUI's own component CSS.
**Owner**: `apps/dashboard/src/index.css` — the single place colour literals may appear.

HeroUI v3 reads these CSS custom properties directly (`@heroui/styles/dist/themes/shared/theme.css`
maps them onto Tailwind utilities via `@theme inline`). Overriding the variables re-themes
both HeroUI components and app markup at once. Names below marked **HeroUI** are fixed by the
library and cannot be renamed; names marked *project* are ours to define.

## Stylesheet shape

```css
@import '@heroui/styles';        /* pulls in tailwindcss + base + components + default theme */

@theme {
  --breakpoint-3xl: 120rem;      /* 1920px — 4 columns */
  --breakpoint-4xl: 160rem;      /* 2560px — 5 columns */
  --container-dashboard: 140rem; /* 2240px content cap (FR-011) */
}

@theme inline {
  /* project-only tokens; HeroUI's own mappings already exist */
  --color-faint: var(--faint);
  --color-border-strong: var(--border-strong);
  --color-accent-soft: var(--accent-soft);
  --color-success-soft: var(--success-soft);
  --color-warning-soft: var(--warning-soft);
  --color-danger-soft: var(--danger-soft);
}

:root, [data-theme='dark'] { /* dark values — default */ }
[data-theme='light']       { /* light values */ }
```

`@import '@heroui/styles'` replaces the current `@import 'tailwindcss'` — it imports Tailwind
itself plus the layer order (`theme, base, components, utilities`).

## Token table

| Token | Source | Role | Dark | Light |
|---|---|---|---|---|
| `--background` | HeroUI | App canvas | near-black | near-white |
| `--background-secondary` / `--background-tertiary` | HeroUI | Derived canvas steps | `color-mix` | `color-mix` |
| `--foreground` | HeroUI | Primary text | near-white | near-black |
| `--surface` | HeroUI | Tile background | raised grey | white |
| `--surface-secondary` | HeroUI | Nested/inset panel (was `--elevated`) | | |
| `--surface-tertiary` | HeroUI | Hover / subtle fill (**was `--overlay`**) | | |
| `--overlay` | HeroUI | Floating surfaces only: modal, popover, tooltip, menu | | |
| `--muted` | HeroUI | Secondary text | | |
| `--faint` | *project* | Tertiary text (HeroUI has only two text tones) | | |
| `--border` | HeroUI | Default hairline | | |
| `--border-strong` | *project* | Emphasised divider | | |
| `--separator` | HeroUI | HeroUI's internal divider | | |
| `--accent` / `--accent-foreground` | HeroUI | The single chromatic hue | same hue both themes | same hue |
| `--accent-soft` | *project* | Selected/active fill | `color-mix(accent 16%)` | `color-mix(accent 12%)` |
| `--focus` | HeroUI | Focus ring — defaults to `var(--accent)`; leave as-is | | |
| `--success` / `--warning` / `--danger` (+`-foreground`) | HeroUI | Status | desaturated | desaturated |
| `--success-soft` / `--warning-soft` / `--danger-soft` | *project* | Status fills | `color-mix(… 14%)` | `color-mix(… 12%)` |
| `--radius` | HeroUI | Tile/control rounding (default `0.5rem`; raise for the reference look) | | |
| `--spacing` | HeroUI | Tailwind spacing base | | |

**Deleted**: `--bg`, `--fg`, `--elevated`, `--primary`, `--primary-fg`, `--primary-soft`, and
the second hue `--accent: #22d3ee`. The body `radial-gradient` background wash and the
`from-primary to-accent` brand gradients are removed (FR-006).

## Migration map

Applied to `apps/dashboard/src/**/*.tsx`. Counts are current occurrences (research R3).

| Old utility | New utility | Count |
|---|---|---|
| `bg-bg` | `bg-background` | few |
| `text-fg` | `text-foreground` | 55 |
| `bg-surface` | `bg-surface` (unchanged) | 9 |
| `bg-elevated` | `bg-surface-secondary` | 23 |
| `bg-overlay` | `bg-surface-tertiary` | 22 |
| `text-muted` | `text-muted` (unchanged) | 79 |
| `text-faint` | `text-faint` (unchanged) | 83 |
| `border-border` | `border-border` (unchanged) | 48 |
| `border-border-strong` | `border-border-strong` (unchanged) | 5 |
| `*-primary` | `*-accent` | ~21 |
| `*-primary-soft` | `*-accent-soft` | ~8 |
| `text-accent`, `to-accent`, `from-primary` | removed (gradients deleted) | ~7 |
| `*-success/warning/danger*` | unchanged | ~40 |

> **`bg-overlay` is the only rename that fails silently.** After the swap it still compiles
> and still resolves — to HeroUI's floating-surface colour, which is wrong in all 22 places.
> All 22 must be rewritten in the same commit as the token definitions.

## Invariants

- **T1** — every neutral token uses `oklch(L 0 0)`; chroma ≠ 0 on a neutral is a defect.
- **T2** — exactly one non-status chromatic token (`--accent`); `--focus` must derive from it.
- **T3** — no colour literal (hex, `rgb()`, `oklch()`, `color-mix()`) and no Tailwind palette
  utility (`bg-zinc-*`, `text-slate-*`, …) outside `index.css`.
- **T4** — both `[data-theme]` blocks define the identical token set.
- **T5** — the accent appears only on: primary buttons, active nav item, focus rings,
  selected/checked states, and single-series data emphasis. Not on tile backgrounds, headers,
  borders, or decorative fills.
- **T6** — dark is the default appearance; the app sets `data-theme` on `<html>` (replacing
  today's `.light` class convention, which is inverted relative to HeroUI).

## Verification

- T1/T2/T4 — parse `index.css` in a vitest unit test; assert chroma and token-set parity.
- T3/T5 — repo-wide grep gate in the same test file (fails with the offending file list).
- T6 + contrast — `tests/e2e/contrast.spec.ts` runs axe with the `color-contrast` rule over
  all nine routes in both appearances (SC-006).
