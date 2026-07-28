# Phase 0 Research: HeroUI Tile-Grid Dashboard Rewrite

**Date**: 2026-07-28 · **Spec**: [spec.md](./spec.md)

All Technical Context unknowns are resolved below. Findings come from inspecting the
published packages (`npm pack @heroui/react@3.2.2`, `@heroui/styles@3.2.2`) and the current
`apps/dashboard` source, not from documentation alone.

---

## R1. Is HeroUI compatible with this stack?

**Decision**: Adopt `@heroui/react@^3.2.2` (HeroUI v3). No stack changes required.

**Evidence**:

```text
@heroui/react 3.2.2
peerDependencies = { react: '>=19.0.0', react-dom: '>=19.0.0', tailwindcss: '>=4.0.0' }
```

Repo today: React 19.0.0, react-dom 19.0.0, tailwindcss 4.0.0, Vite 6, TS 5.6 — every peer
is already satisfied at the required major. HeroUI v3 is built on React Aria + Tailwind v4,
ships no CSS-in-JS runtime, and needs **no `<Provider>` wrapper**, so `app/providers.tsx`
is untouched.

**Rationale**: The riskiest assumption in the spec (that "HeroUI" is even installable here)
is confirmed cheaply and first. v2 would have been a problem — it is a Tailwind **v3**
plugin — but v3 is Tailwind-v4-native.

**Alternatives considered**: HeroUI v2 (rejected: Tailwind v3 plugin, would force a
downgrade); shadcn/ui copy-paste (rejected: user explicitly named HeroUI); keeping the
bespoke layer and only restyling (rejected in clarification Q1).

---

## R2. How does HeroUI theming work, and can it express a monochrome palette?

**Decision**: Re-value HeroUI's CSS custom properties in `index.css`. No plugin config, no
JS theme object, no fork of the library styles.

**Evidence** — `@heroui/styles/dist/themes/shared/theme.css` is a plain `@theme inline`
block mapping Tailwind utilities onto CSS vars, structurally identical to what the repo
already does:

```css
@theme inline {
  --color-background: var(--background);
  --color-surface: var(--surface);
  --color-muted: var(--muted);
  --color-accent: var(--accent);
  --color-border: var(--border);
  --color-focus: var(--focus);
  --color-success: var(--success);  /* + warning, danger, overlay, field, … */
}
```

and `themes/default/variables.css` defines the values under `:root, .light, [data-theme="light"]`
with a matching `.dark, [data-theme="dark"]` block (line 177). Values are `oklch()`, and
derived tokens use `color-mix(in oklab, …)`.

**Consequences**:

- Monochrome = set every colour var to `oklch(L 0 0)` (chroma 0) and give `--accent` the one
  chromatic hue. `--focus` already defaults to `var(--accent)`, satisfying FR-013 for free.
- `--radius` (default `0.5rem`) and `--spacing` are themeable, so tile rounding is a token.
- Dark mode switches on `.dark` / `[data-theme="dark"]`. The repo currently uses `.light` on
  `<html>` with dark as the default — **inverted** relative to HeroUI. The app must switch to
  HeroUI's convention: `data-theme="dark"` as the default attribute.
- HeroUI also supports `data-vibrant-palette`; unused here.

**Alternatives considered**: Wrapping HeroUI components in project-styled shells (rejected:
duplicates a theming layer that already exists); overriding with `!important` utilities
(rejected: fights the cascade, breaks `color-mix` derivations).

---

## R3. Token name collision with the existing stylesheet

**Decision**: Adopt HeroUI's token vocabulary as canonical and migrate the repo's names to
it. Three project-specific tokens survive because HeroUI has no equivalent.

The overlap is high but **not** identical, and one name means two different things:

| Current repo token | HeroUI token | Action |
|---|---|---|
| `--bg` | `--background` | rename |
| `--surface` | `--surface` | same name, re-valued |
| `--elevated` | `--surface-secondary` | rename |
| `--overlay` (a hover/elevated grey) | `--surface-tertiary` | **rename — collision** |
| — | `--overlay` (floating: modals, popovers, tooltips) | new meaning |
| `--border` | `--border` | same |
| `--border-strong` | — | keep as project token |
| `--fg` | `--foreground` | rename |
| `--muted` | `--muted` | same |
| `--faint` | — | keep as project token (HeroUI has only two text tones) |
| `--primary`, `--primary-fg` | `--accent`, `--accent-foreground` | rename |
| `--primary-soft` | — | keep as project token (`color-mix` of accent) |
| `--accent` (cyan, second hue) | — | **delete** — FR-005 allows exactly one accent |
| `--success/--warning/--danger` | same names | re-valued, subdued |
| `--success-soft` etc. | — | keep as project tokens |

**Blast radius** (measured with `grep` over `apps/dashboard/src`): 83 `text-faint`, 79
`text-muted`, 55 `text-fg`, 48 `border-border`, 23 `bg-elevated`, 22 `bg-overlay`, plus
~60 accent/status utilities. `text-muted` and `border-border` need no change; `text-fg`,
`bg-elevated`, `bg-overlay`, and every `*-primary*` do.

**`bg-overlay` is the trap**: 22 call sites currently mean "slightly raised grey"; under
HeroUI the same utility will resolve to the floating-surface colour. These must be rewritten
to `bg-surface-tertiary` in the same commit that swaps the token definitions, or they will
silently render the wrong shade rather than fail a build.

**Alternatives considered**: Keeping repo names and aliasing them onto HeroUI vars (rejected:
two vocabularies forever, and the `--overlay` collision stays live); renaming HeroUI's vars
(impossible — component CSS references them directly).

---

## R4. What is actually being replaced?

**Decision**: The swap is far smaller than the dependency list suggests.

Six `@radix-ui/*` packages are declared, but only **two are imported anywhere**:

```text
src/components/toast.tsx                        -> @radix-ui/react-toast
src/features/profile/components/ConfirmDialog.tsx -> @radix-ui/react-dialog
```

`react-select`, `react-switch`, `react-tabs`, `react-tooltip`, `react-slot` are **unused
dependencies** — they can be deleted outright with no code change. Selects, switches, and
checkboxes today are native elements wrapped in `components/ui.tsx` (234 lines, 18 exports).

Every replacement exists in HeroUI v3's export map: `./toast`, `./modal`, `./alert-dialog`,
`./select`, `./switch`, `./tabs`, `./tooltip`, `./card`, `./surface`, `./skeleton`,
`./spinner`, `./empty-state`, `./scroll-shadow`, `./table`, `./input`, `./textarea`,
`./checkbox`, `./button`, `./chip`/`./tag`, `./separator`, `./progress-bar`.

**Retained**: `@dnd-kit/*` (profile section reordering — 2 files) and
`@tanstack/react-virtual` (`VirtualList.tsx`); HeroUI has no equivalent for either, per
clarification Q1. `clsx` / `tailwind-merge` stay (HeroUI bundles its own `tailwind-merge`
copy — a duplicate version in the tree is acceptable and not a conflict).

**Caveat worth recording**: HeroUI v3 itself depends on `@radix-ui/react-avatar@1.1.11`.
"Radix removed" therefore means removed from **direct** dependencies; one Radix package
remains transitively and that is out of our control.

---

## R5. Breakpoints for 4- and 5-column layouts

**Decision**: Add two custom breakpoints via Tailwind v4's `@theme`; use the built-ins below
them.

```css
@theme {
  --breakpoint-3xl: 120rem; /* 1920px */
  --breakpoint-4xl: 160rem; /* 2560px */
}
```

Mapping to FR-001 (Tailwind v4 defaults: sm 640, md 768, lg 1024, xl 1280, 2xl 1536):

| Width | Columns | Utility |
|---|---|---|
| <640 | 1 | `grid-cols-1` |
| 640–1023 | 2 | `sm:grid-cols-2` |
| 1024–1919 | 3 | `lg:grid-cols-3` |
| 1920–2559 | 4 | `3xl:grid-cols-4` |
| ≥2560 | 5 | `4xl:grid-cols-5` |

Note `md` (768) is deliberately skipped by the grid — it stays reserved for the shell's
sidebar switch, which already uses `md:` and a `(max-width: 767.98px)` media query.

**Width cap (FR-011)**: `max-width: 140rem` (2240px) centred. With 5 columns and a 1.25rem
gap that yields ~430px tiles — above the ~360px where KPI tiles start looking sparse and
below the ~55ch where prose becomes hard to track.

**Alternatives considered**: `grid-template-columns: repeat(auto-fit, minmax(…))` (rejected:
column count becomes emergent, so FR-001's exact counts can't be asserted); container
queries (rejected: the grid tracks the viewport, not a resizable container, and per-tile
container queries are still useful *inside* tiles later).

---

## R6. Hybrid scroll model (FR-020)

**Decision**: A `variant` on the grid plus a shell layout mode, selected per route.

- `flow` (default, content-heavy pages): shell `<main>` scrolls normally; tiles size to
  content. Applies to job detail, profile, tailor, settings, sources, contacts.
- `fit` (overview pages: feed, status, tracker): at ≥1024px **and** viewport height ≥45rem,
  `<main>` is `h-[100dvh] overflow-hidden` and the grid uses `grid-rows-*` with `min-h-0`
  tiles that scroll internally.
- Below 1024px, or on short viewports, `fit` degrades to `flow` — this is the FR-020 /
  short-viewport edge case, expressed as `@media (min-width:1024px) and (min-height:45rem)`.

`min-h-0` on grid children is mandatory; without it a `overflow-y-auto` child of a grid row
refuses to shrink and pushes the row past the viewport (default `min-height: auto`). This is
the single most likely implementation bug in the whole feature.

**Internal scroll a11y (FR-021)**: HeroUI's `ScrollShadow` provides the "more content below"
affordance. Keyboard reachability requires the scroll container to be focusable —
`tabIndex={0}` with `role="region"` and an `aria-label` — since a scrollable `div` with no
focusable content is unreachable by keyboard in Firefox and Safari.

**Alternatives considered**: Fit-to-viewport everywhere (rejected in clarification Q5 —
crushes job detail and profile); page scroll everywhere (rejected: loses the reference look
on the overview screens).

---

## R7. How the layout criteria get verified

**Decision**: Extend the existing Playwright suite; keep component tests in vitest.

`apps/dashboard/playwright.config.ts` already exists (chromium, `baseURL localhost:5173`,
`webServer: pnpm dev`, `testDir ./tests/e2e`) with three specs: `feed`, `navigation`,
`sources`. A new `tests/e2e/layout.spec.ts` iterates the viewport matrix
(375, 768, 1280, 1920, 2560, 3840 — plus a 320 minimum-width check) across all nine routes
and asserts:

- computed `grid-template-columns` track count equals the FR-001 expectation (SC-001);
- `document.documentElement.scrollWidth <= clientWidth` (SC-004);
- on `fit` pages at 1920/2560, `scrollHeight <= clientHeight` (SC-010).

Contrast (SC-006) needs a new devDependency, `@axe-core/playwright`, run per page per theme
with the `color-contrast` rule enabled. This is the only new test-tooling dependency.

Why not vitest/jsdom: jsdom implements no layout engine — `getComputedStyle` returns the
declared value, `offsetWidth` is always 0, and grid tracks are never resolved. Column and
overflow assertions there would pass vacuously. The 24 existing vitest files keep covering
behaviour and the per-widget loading/empty/error states (SC-007, FR-009).

**Alternatives considered**: Screenshot baselines (rejected in clarification Q3 — snapshot
churn across 9 pages × 6 viewports × 2 themes); manual checklist (rejected: SC-001/004/010
would not be enforceable).

---

## R8. Bundle-size and performance posture

**Decision**: Import from HeroUI subpaths (`@heroui/react/button`), not the barrel, and
record a before/after bundle delta as a plan artifact.

The package exposes ~85 per-component subpath exports plus per-component npm packages.
Barrel imports risk pulling the whole React Aria surface into the initial chunk. Deleting
`components/ui.tsx` and five unused Radix packages offsets part of the addition.

**Budget**: record `pnpm --filter @job-finder/dashboard build` output size before the
rewrite; the post-rewrite initial JS chunk (gzipped) must not exceed baseline + 100 KB, and
any regression beyond that is reported rather than silently accepted. The spec sets no
runtime performance target, and none is invented here — this is a build-artifact gate only.

---

## Resolved unknowns summary

| Unknown | Status |
|---|---|
| HeroUI version / peer compatibility | Resolved (R1) — v3.2.2, all peers met |
| Theming mechanism vs. existing `@theme inline` | Resolved (R2) — same mechanism, re-value vars |
| Token collisions | Resolved (R3) — `--overlay` semantics change; migration table fixed |
| Scope of the component swap | Resolved (R4) — 2 real call sites, 5 unused deps |
| 4/5-column breakpoints | Resolved (R5) — custom `3xl`/`4xl`, 140rem cap |
| Fit-vs-flow implementation | Resolved (R6) — grid `variant` + shell mode + `min-h-0` |
| Verification strategy | Resolved (R7) — Playwright matrix + axe; vitest for states |
| Performance/bundle | Resolved (R8) — subpath imports, +100 KB gz budget |
