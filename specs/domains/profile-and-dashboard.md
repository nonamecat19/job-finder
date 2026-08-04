# Domain: Profile & Dashboard UI

Consolidates **009** editable resume profile, **021** HeroUI tile-grid rewrite,
**006** skeleton loading states, **001-global-dashboard-grid** (superseded by 021).

Implementation: `apps/dashboard/src/`, `apps/api/internal/profile/`. How it works:
[`docs/frontend/overview.md`](../../docs/docs/frontend/overview.md),
[`docs/frontend/component-system.md`](../../docs/docs/frontend/component-system.md).

Nine pages: feed, job detail, tracker, tailor, contacts, sources, status, profile, settings.

---

## 1. Editable resume profile (009)

The Profile tab is a full structured editor, not an import viewer.

| # | Requirement |
|---|---|
| 009-FR-001 | A user can create and fully populate a resume — name, headline, contact/location/social links, and all sections — directly in the Profile tab. |
| 009-FR-002 | Uploading a config file is **optional**, and is only a pre-fill convenience. **No functionality may require a config file to be present.** |
| 009-FR-003 | A dedicated structured form exists for every RenderCV entry type: experience, education, skills, projects, certifications, publications, links, and the rest. |
| 009-FR-004 | Add, edit and delete individual entries within any section. |
| 009-FR-005 | Add, rename and delete whole sections, **including custom sections** with names outside the predefined type list. |
| 009-FR-006 | Reorder entries within a section and reorder sections themselves; the order persists. This order is what `resume-generation.md` § 3 preserves. |
| 009-FR-007 | Inline validation (required fields, date ordering) before save, with field-specific messages that do not discard the user's input. |
| 009-FR-008 | Saves are reliable, with clear visual confirmation of success or failure (009-SC-004: edits survive reload 100% of the time). |
| 009-FR-009 | Resume data in an uploaded config that matches no recognised entry type is **preserved and surfaced** through a fallback editor — never silently dropped (009-SC-002: zero silent data loss). |
| 009-FR-010 | Uploading a new config over existing content warns that content will be replaced and requires confirmation. |
| 009-FR-011 | Deleting an entry or a section requires explicit confirmation (009-SC-005: verified across **all** delete paths). |
| 009-FR-012 | A profile with zero sections and zero fields is a valid, non-error state. |
| 009-FR-013 | Clean layout with clear visual hierarchy, distinguishing section-level from entry-level actions. |

Bar: a complete multi-section resume can be built entirely by hand in under 15 minutes
(009-SC-001), and controls are discoverable without instructions (009-SC-003).

### 1.1 The resume API

Extends `ProfilesHandler` (`internal/httpapi/profiles.go`). The pre-existing routes —
`GET`/`POST /profiles`, `GET`/`PUT`/`DELETE /profiles/{id}`, `POST /profiles/config`,
`GET /profiles/config/status` — are unchanged, and config upload remains a supported optional
pre-fill path (009-FR-002).

| Method | Path | Behaviour |
|---|---|---|
| `GET` | `/profiles/{id}/resume` | `200 {resume}` — the structured `Resume` derived from the profile's current `rendercvConfig`/`rendercvYaml`, with unrecognised data included under each `unrecognized` field rather than dropped. `404` if the profile does not exist. |
| `PUT` | `/profiles/{id}/resume` | `200 {resume}` with the **re-read persisted state**, so the client always shows the authoritative value. `400` on validation failure. `404` if the profile does not exist. |

**An empty `Resume` is a `200`, not an error.** A profile with no config yet returns
`sections: []` with only `name` populated — or even empty. That is 009-FR-012's
"start from scratch" state.

**A `400` carries a machine-readable field path**, e.g. `sections[2].entries[0].endDate`, so
the client can point at the offending field instead of showing a generic message
(009-FR-007).

**There is exactly one write path.** `PUT` replaces the whole document; there is no
partial-field PATCH and there are no per-entry or per-section mutation endpoints. Adding,
editing, deleting and reordering all happen client-side against the in-memory `Resume`, then
save in one `PUT`. This matches the existing whole-blob persistence model and keeps the
mapping layer the single place where order and round-trip correctness are enforced. The write
path is `Resume` → `RendercvMaster` map (`resume_mapping.go`) → YAML via
`PrepareMasterForMarshal`, which is what preserves section order (009-FR-006), then the
existing `rendercvYaml`/`rendercvConfig` update.

**The overwrite warning is the client's job** (009-FR-010). The server has no UI layer and
does not prompt; `GET /profiles/config/status` may carry a `hasExistingContent: true`
advisory flag so the client knows to show the confirmation dialog before calling
`POST /profiles/config`.

## 2. Tile grid and design system (021)

021 replaced the earlier grid (001-global-dashboard-grid) with a tile system covering every
page. **021 governs; 001-global-dashboard-grid is historical.** See § 4.

**Grid**

- 021-FR-001: 1 column below 640 px, 2 at 640–1023, then progressively up to 5 columns.
- 021-FR-002: tiles span multiple columns and/or rows, degrading predictably at each smaller
  breakpoint.
- 021-FR-003: grid gaps, tile radius, tile padding and border treatment are defined **once**
  as shared tokens. No per-page overrides.
- 021-FR-004: all nine pages are built on the tile grid (021-SC-003: zero page-specific
  layout CSS remains).
- 021-FR-010: no horizontal page scroll at any width from 320 px to 3840 px.
- 021-FR-011: above the widest breakpoint, total content width is capped and centred.
- 021-FR-017: virtualized surfaces keep virtualizing, with the containing tile providing a
  bounded height.
- 021-FR-018: a new page or widget can be added using the shared primitives with **no**
  page-specific layout CSS.

**Scroll model (021-FR-020, 021-FR-021)**

At ≥1024 px the dashboard is hybrid: overview pages (feed, status, tracker) fit the viewport
with no page-level scroll (021-SC-010, verified at 1920 px and 2560 px); tiles that scroll
internally keep that region keyboard-reachable and indicate that more content exists below
the fold.

**Colour**

- 021-FR-005: monochrome-first. Background, surface, elevated surface, border and text tokens
  are all neutral greyscale, plus exactly one accent.
- 021-FR-006: the accent is restricted to interactive emphasis — primary buttons, active nav,
  focus rings, selection — and single-series data emphasis.
- 021-FR-007: semantic success/warning/danger stay available and distinguishable, in a
  subdued form consistent with the monochrome system.
- 021-FR-008: dark and light appearances share identical layout, differing only in neutral
  token values.
- 021-SC-005: 100% of colour values resolve to the shared token set; zero hard-coded colours
  outside the token definitions.

**Accessibility**

- 021-FR-013: every interactive element has a visible focus indicator using the accent token,
  and all tiles and controls stay keyboard-navigable (021-SC-009).
- 021-FR-014: text and essential UI meet WCAG AA contrast against their tile surface in
  **both** appearances (021-SC-006, automated check).
- 021-FR-015: motion is subtle and disabled under `prefers-reduced-motion`.

**Constraints on the rewrite**

- 021-FR-016: presentation-layer only. No change to API contracts, data-fetching semantics or
  business logic; existing behaviour tests pass unmodified (021-SC-008).
- 021-FR-012: the app shell — sidebar nav, mobile nav, notification affordance, page header —
  adopts the new language while preserving its behaviour.
- 021-FR-019: **one** component foundation. No two overlay/focus-management systems may
  coexist after the rewrite. Adding a second UI kit reopens this requirement.

### 2.1 The layout primitives

`apps/dashboard/src/components/layout/`. **These are the only layout APIs a page author may
use.** A page that needs layout CSS beyond these props means the contract is wrong — fix the
primitive, not the page (021-FR-018).

**`DashboardGrid`** takes a `variant` of `'flow'` (default) or `'fit'`, inherited from the
route's layout mode rather than passed by the page.

| Aspect | Value |
|---|---|
| Columns | 1 (<640) · 2 (640–1023) · 3 (1024–1919) · 4 (1920–2559) · 5 (≥2560) |
| Gap | `0.75rem` <640 · `1rem` 640–1023 · `1.25rem` ≥1024 |
| Max width | `140rem`, centred |
| Alignment | `items-start` in `flow`, `items-stretch` in `fit` |
| Rows | `fit` sets explicit row tracks so tiles divide the viewport height; `flow` uses implicit rows |

The `3xl`/`4xl` breakpoints come from the token contract — never hardcode pixel media queries
in the component. `fit` is gated on `@media (min-width: 1024px) and (min-height: 45rem)`;
below **either** threshold it degrades to `flow`, which is how the short-viewport case is
handled. The grid must not set `overflow` — page-level scrolling is the shell's concern.

**`Tile`** replaces both `GridCard` (spacing) and `Surface` (chrome) — one component, not two.
Its props: `title`, `action`, `footer`, `span`, `tone` (`default | inverse | quiet`), `scroll`
+ `scrollLabel`, `state` (`ready | loading | empty | error`), `emptyMessage`, `error`,
`onRetry`, `children`.

Four guarantees carry the design:

- **The grid footprint is state-independent.** `loading`, `empty` and `error` occupy exactly
  the cells `span` allocates. **Never conditionally render a `Tile` away — pass a state
  instead.** This is 021-FR-009 / 021-SC-007 made mechanical.
- **`min-w-0` and `min-h-0` are always applied**, so a tile can shrink inside its track. This
  is what stops an `overflow-y-auto` child from blowing past the viewport in `fit` mode.
- **`scroll: true` wraps the body in a focusable region** — `tabIndex={0}`, `role="region"`,
  `aria-label={scrollLabel}` — with a visible overflow affordance. Without `tabIndex`, a
  scrollable div with no focusable children is **keyboard-unreachable in Firefox and Safari**;
  that is why `scrollLabel` is required rather than optional (021-FR-021).
- **Styling is closed.** `className` may adjust internal body layout (flex direction, gap) but
  must not set background, border, radius, padding or colour. Those come from tokens.

`tone="inverse"` is the high-contrast tile — it inverts `--foreground`/`--background` and
**never uses the accent**.

Span translation is owned by the primitive, not the caller:

| `span` | 5col | 4col | 3col | 2col | 1col |
|---|---|---|---|---|---|
| `compact` / `standard` | 1×1 | 1×1 | 1×1 | 1×1 | 1×1 |
| `wide` | 2×1 | 2×1 | 2×1 | 2×1 | 1×1 |
| `tall` | 1×2 | 1×2 | 1×2 | 1×2 | 1×1 |
| `feature` | 3×2 | 2×2 | 2×2 | 2×1 | 1×1 |
| `full` | 5×1 | 4×1 | 3×1 | 2×1 | 1×1 |

Row spans are enforced in `fit` and advisory (`min-height`) in `flow`.

`TileSkeleton`, `TileEmpty` and `TileError` are the internal state presentations, exported for
panels rendering sub-regions. **`TileError` never rethrows** — a failing widget stays contained
to its own cell.

`PageHeader` keeps its `{title, description?, actions?}` API, restyled, with one constraint:
total rendered height stays within one grid row, so the description truncates rather than
wrapping to a third line at desktop widths.

**Layout mode is a route property, not a tile property.** `fit` for `/`, `/status` and
`/tracker`; `flow` for the rest; an undeclared route defaults to `flow`. The shell resolves it
once and applies it to `<main>` — `fit` → `lg:h-[100dvh] lg:overflow-hidden` (gated on the
min-height query), `flow` → normal document scroll. **A tile cannot override the mode**; a
page needing a different one changes its route declaration.

What the rewrite removed, and what replaced it:

| Removed | Replacement |
|---|---|
| `GridCard` | `Tile` (`narrow`→`standard`, `wide`→`wide`, `full`→`full`) |
| `Surface` (from `ui.tsx`) | `Tile` chrome |
| `LoadingRegion`, `SkeletonLine`/`Block`/`Circle` | `Tile state="loading"` / `TileSkeleton` |
| `EmptyState`, `ErrorState` | `Tile state="empty" \| "error"` / `TileEmpty`, `TileError` |
| `Button`, `Input`, `Textarea`, `Select`, `Checkbox`, `Field`, `Chip`, `Spinner` | HeroUI subpath imports |
| `ScoreBadge`, `GhostBadge`, `HealthDot` | Kept as app-specific components, restyled onto tokens + HeroUI `Chip`/`Tag` |

**Import HeroUI from subpaths (`@heroui/react/button`), never the barrel** — the barrel blows
the bundle budget.

### 2.2 The token contract

`apps/dashboard/src/index.css` is **the single place a colour literal may appear.** HeroUI
reads these CSS custom properties directly and maps them onto Tailwind utilities, so
overriding the variables re-themes HeroUI components and app markup at once.

```css
@import '@heroui/styles';        /* replaces @import 'tailwindcss'; brings the layer order too */

@theme {
  --breakpoint-3xl: 120rem;      /* 1920px — 4 columns */
  --breakpoint-4xl: 160rem;      /* 2560px — 5 columns */
  --container-dashboard: 140rem; /* 2240px content cap */
}
```

Tokens fixed by HeroUI: `--background` (+ `-secondary`/`-tertiary`), `--foreground`,
`--surface` (tile background), `--surface-secondary` (nested/inset panel),
`--surface-tertiary` (hover/subtle fill), `--overlay` (**floating surfaces only** — modal,
popover, tooltip, menu), `--muted`, `--border`, `--separator`, `--accent` /
`--accent-foreground`, `--focus`, `--success`/`--warning`/`--danger` (+ `-foreground`),
`--radius`, `--spacing`. Project-defined additions: `--faint` (tertiary text — HeroUI ships
only two text tones), `--border-strong`, `--accent-soft`, and
`--success-soft`/`--warning-soft`/`--danger-soft`.

Deleted by 021: `--bg`, `--fg`, `--elevated`, `--primary`, `--primary-fg`, `--primary-soft`,
and the **second hue** `--accent: #22d3ee` — along with the body `radial-gradient` wash and
the `from-primary to-accent` brand gradients.

Six invariants, all machine-checked:

| # | Invariant | Checked by |
|---|---|---|
| T1 | Every neutral token is `oklch(L 0 0)` — non-zero chroma on a neutral is a defect | vitest parses `index.css` |
| T2 | Exactly one non-status chromatic token (`--accent`); `--focus` derives from it | same |
| T3 | No colour literal (hex, `rgb()`, `oklch()`, `color-mix()`) and no Tailwind palette utility (`bg-zinc-*`, …) outside `index.css` | repo-wide grep gate |
| T4 | Both `[data-theme]` blocks define the identical token set | vitest |
| T5 | The accent appears **only** on primary buttons, active nav, focus rings, selected/checked states and single-series data emphasis — never on tile backgrounds, headers, borders or decorative fills | grep gate |
| T6 | Dark is the default; `data-theme` is set on `<html>`, replacing the old `.light` class convention (which was inverted relative to HeroUI) | `tests/e2e/contrast.spec.ts`, which also runs axe's `color-contrast` rule over all nine routes in both appearances |

> **`bg-overlay` was the only rename that fails silently.** After the token swap it still
> compiles and still resolves — to HeroUI's floating-surface colour, which is wrong in all 22
> places it appeared. Renames like this must land in the same commit as the token definitions.

**Bundle budget.** The pre-rewrite baseline (2026-07-28) was 182.55 KB initial JS gzipped,
7.64 KB CSS, with 204 tests across 24 files all passing. The rewrite's ceiling was baseline +
100 KB — **282.55 KB gzipped**.

## 3. Loading, empty and error states (006, 021-FR-009)

- 021-FR-009: **every** tile defines loading, empty and error presentations, and each
  preserves the tile's grid footprint (021-SC-007, verified per widget).
- 006-FR-001: one shared skeleton primitive family — text line, block/card, avatar/circle.
- 006-FR-002: every page and panel that previously used a spinner or inline "Loading…" text
  uses it instead (006-SC-001: 100%).
- 006-FR-003: a skeleton approximates the count, size and arrangement of the content it
  precedes, so swapping in real content causes no layout shift (006-SC-002: zero measured
  shift on feed, tracker, job detail).
- 006-FR-004: the skeleton is replaced by real content as soon as data arrives, and by the
  error state when the request fails. 006-SC-004: **skeleton and error never render
  simultaneously** on any page.
- 006-FR-005: animation timing and base styling are identical across every feature area.
- 006-FR-006: a scoped reload (list re-fetch on filter change) re-skeletons only that
  section, without remounting the page.
- 006-FR-007: skeletons are not announced by screen readers as real content, and loading
  status is still conveyed to assistive technology.

## 4. Superseded: 001-global-dashboard-grid

001-global-dashboard-grid specified a 3-column grid (1 below 768 px, 2 at 768–1023, 3 at
≥1024) with a 1.25 rem desktop gap, applied to all nine pages. 021 replaced every one of
those numbers: up to 5 columns, different breakpoints, tokenised gaps, and a hybrid scroll
model 001 did not contemplate.

Two of 001's rules survive as principles and are restated in 021:

- 001-FR-008 → 021-SC-008: no functionality or content is lost to a layout change.
- 001-FR-010 → 021-FR-017: virtualized lists keep virtualizing.

Do not cite 001-global-dashboard-grid's breakpoints or gap values. They are wrong now.
