# Phase 1 Data Model: HeroUI Tile-Grid Dashboard Rewrite

**Date**: 2026-07-28 · **Spec**: [spec.md](./spec.md) · **Research**: [research.md](./research.md)

This feature persists nothing. "Entities" here are the design-time structures that page
authors work with — the vocabulary that `tasks.md` and the implementation must share. No
database table, API field, or `packages/shared` type changes.

---

## E1. Theme Token Set

The palette contract. Values live in `apps/dashboard/src/index.css`; the full name-by-name
mapping is in [contracts/theme-tokens.md](./contracts/theme-tokens.md).

| Group | Tokens | Rule |
|---|---|---|
| Neutrals | `background`, `background-secondary/tertiary`, `surface`, `surface-secondary`, `surface-tertiary`, `overlay`, `foreground`, `muted`, `faint`, `border`, `border-strong`, `separator` | Chroma must be 0 (`oklch(L 0 0)`) — greyscale only |
| Accent | `accent`, `accent-foreground`, `accent-soft`, `focus` | Exactly one chromatic hue; `focus` derives from `accent` |
| Status | `success`, `warning`, `danger` (+ `-foreground`, `-soft`) | Chromatic but desaturated; must stay distinguishable from `accent` |
| Shape | `radius`, `field-radius`, `spacing`, `border-width` | Single source for tile rounding and rhythm |

**Validation rules**

- V1: every neutral token has chroma 0 — automatable by parsing `index.css` for `oklch()`
  values in the neutral group (FR-005, SC-005).
- V2: exactly one token is the accent hue; a second chromatic non-status token is a defect
  (the current `--accent: #22d3ee` alongside `--primary: #7c6cff` is what this removes).
- V3: no colour literal appears outside `index.css` — no hex, `rgb()`, `oklch()`, or Tailwind
  palette utility (`bg-zinc-800`, `text-slate-400`) in any `.tsx` (SC-005).
- V4: both appearances define every token; a token present in one theme block and missing in
  the other is a defect (FR-008).

**States**: two appearances, `light` and `dark`, selected by `data-theme` on `<html>`.
Dark is the default. Layout and spacing are identical between them (FR-008).

---

## E2. Tile (Widget)

The universal content unit. One tile = one grid cell (or a spanned block of cells).

| Field | Type | Notes |
|---|---|---|
| `title` | string \| ReactNode, optional | Omitted for bare/figure tiles |
| `action` | ReactNode, optional | Header-right affordance (link, menu, toggle) |
| `footer` | ReactNode, optional | Secondary metadata row |
| `span` | `SpanRule` (E3) | Defaults to `standard` |
| `tone` | `'default' \| 'inverse' \| 'quiet'` | `inverse` is the reference look's black-tile-on-light-field emphasis; achieved with neutral tokens, never the accent (FR-006) |
| `scroll` | boolean | When true the body scrolls internally; mandatory for fit-mode tiles whose content can exceed their height |
| `state` | `'ready' \| 'loading' \| 'empty' \| 'error'` | Drives which body renders |

**Validation rules**

- V5: a tile renders its grid cell in **every** state — loading, empty, and error must not
  collapse the tile or change its span (FR-009, SC-007).
- V6: `scroll: true` requires the scroll container to be keyboard-focusable (`tabIndex={0}`,
  `role="region"`, `aria-label`) and to show an overflow affordance (FR-021).
- V7: a tile never sets its own colours, radius, padding, or gap — all come from E1 (FR-003).
- V8: tile content must not force horizontal overflow; long strings wrap or truncate, wide
  content (tables, code) scrolls inside the tile (FR-010).

**State transitions**

```text
loading ──data──▶ ready
   │                │
   │                └──refetch──▶ loading
   ├──no rows──────▶ empty ──refetch──▶ loading
   └──failure──────▶ error ──retry───▶ loading
```

`error` always offers a retry affordance and never blanks the surrounding page — a failing
widget is contained to its own cell (Edge Cases).

---

## E3. Span Rule

Semantic size classification. Page authors pick a name; the primitive owns the per-breakpoint
column/row translation, so FR-002's degradation rule can never be violated by a caller.

| Name | ≥2560 (5col) | 1920–2559 (4col) | 1024–1919 (3col) | 640–1023 (2col) | <640 |
|---|---|---|---|---|---|
| `compact` | 1×1 | 1×1 | 1×1 | 1×1 | 1×1 |
| `standard` | 1×1 | 1×1 | 1×1 | 1×1 | 1×1 |
| `wide` | 2×1 | 2×1 | 2×1 | 2×1 | 1×1 |
| `tall` | 1×2 | 1×2 | 1×2 | 1×2 | 1×1 |
| `feature` | 3×2 | 2×2 | 2×2 | 2×1 | 1×1 |
| `full` | 5×1 | 4×1 | 3×1 | 2×1 | 1×1 |

`compact` and `standard` share a footprint but differ in internal density — `compact` is the
KPI treatment (large numeral, label, context line) required by User Story 3 scenario 2.

**Validation rules**

- V9: a column span never exceeds the column count at that breakpoint (FR-002).
- V10: row spans apply only in fit mode; in flow mode tiles size to content and a `tall`/
  `feature` row span is advisory (`min-height`), not a clamp.

---

## E4. Page Layout Mode

Fixed classification from clarification Q5; not user-configurable (FR-020).

| Mode | Routes | Behaviour at ≥1024px & ≥45rem height | Below that |
|---|---|---|---|
| `fit` | `/` (feed), `/status`, `/tracker` | `<main>` is viewport-height, no page scroll; tiles take fixed heights and scroll internally | Falls back to `flow` |
| `flow` | `/jobs/:id`, `/profile`, `/tailor`, `/settings`, `/sources`, `/contacts` | Page scrolls; tiles size to content | Same |

**Validation rules**

- V11: a `fit` page produces no page-level vertical scroll at 1920px and 2560px (SC-010).
- V12: every route declares a mode; an undeclared route defaults to `flow` (the safe mode —
  it can never clip content).
- V13: mode is a property of the route, resolved once in the shell; individual tiles cannot
  override it.

---

## E5. Grid Container

| Field | Value |
|---|---|
| Columns | 1 / 2 / 3 / 4 / 5 at <640 / 640 / 1024 / 1920 / 2560 (FR-001) |
| Gap | 0.75rem <640 · 1rem 640–1023 · 1.25rem ≥1024 |
| Alignment | `items-start` in flow mode; `items-stretch` in fit mode |
| Max width | 140rem (2240px), centred (FR-011) |
| Variant | `flow` \| `fit` — inherited from E4 |

**Validation rules**

- V14: computed `grid-template-columns` track count equals the table above at every viewport
  in the test matrix (SC-001).
- V15: `document.documentElement.scrollWidth ≤ clientWidth` at every tested width from 320px
  to 3840px (SC-004, FR-010).
- V16: adding a page requires no layout CSS beyond choosing a mode and per-tile span rules
  (FR-018, SC-004).

---

## Relationships

```text
Theme Token Set (E1)
   └─ consumed by ─▶ Tile (E2) ─ classified by ─▶ Span Rule (E3)
                        │
                        └─ placed in ─▶ Grid Container (E5)
                                            │
                                            └─ variant from ─▶ Page Layout Mode (E4)
```

One token set → many tiles. One grid per page. One mode per route. Span rules are a closed
enumeration shared by all tiles.
