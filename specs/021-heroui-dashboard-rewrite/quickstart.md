# Quickstart: Validating the HeroUI Tile-Grid Rewrite

**Spec**: [spec.md](./spec.md) · **Plan**: [plan.md](./plan.md) · **Contracts**: [theme-tokens](./contracts/theme-tokens.md), [layout-primitives](./contracts/layout-primitives.md)

How to run and prove the rewrite. Every scenario maps to a success criterion; run them in
order — later ones assume the app builds.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build     # required before dashboard tooling
make up                                     # Postgres/Redis/API for real data
```

Dashboard dev server: `pnpm --filter @job-finder/dashboard dev` → http://localhost:5173

## Step 0 — Record the bundle baseline (before any code changes)

```bash
pnpm --filter @job-finder/dashboard build
```

Save the reported gzipped size of the initial JS chunk. The post-rewrite build must land
within **baseline + 100 KB** (plan, research R8). Without this number captured first, the
budget is unverifiable.

## Step 1 — Type and unit gate

```bash
pnpm --filter @job-finder/dashboard typecheck
pnpm --filter @job-finder/dashboard test
```

**Expected**: clean typecheck; all vitest files pass. Covers SC-008 (no capability lost) and
SC-007 (per-widget loading/empty/error state tests).

## Step 2 — Token audit

```bash
pnpm --filter @job-finder/dashboard test -- token
```

The token unit test asserts contract invariants T1–T4 by parsing `src/index.css` and grepping
the source tree:

- every neutral token is `oklch(L 0 0)` (chroma 0);
- exactly one non-status chromatic token exists;
- both `[data-theme]` blocks define the same token set;
- no colour literal or Tailwind palette utility outside `index.css`.

**Expected**: pass, satisfying SC-005. A failure prints the offending file and token.

Manual companion check — no direct Radix imports survive:

```bash
grep -rn "@radix-ui" apps/dashboard/src   # expect: no matches
grep -rn "bg-overlay" apps/dashboard/src  # expect: only floating components, if any
```

## Step 3 — Layout matrix

```bash
pnpm --filter @job-finder/dashboard test:e2e -- layout
```

`tests/e2e/layout.spec.ts` visits all nine routes at 320, 375, 768, 1280, 1920, 2560 and
3840 px and asserts per viewport:

| Assertion | Criterion |
|---|---|
| computed `grid-template-columns` track count = 1/2/3/4/5 per FR-001 | SC-001 |
| `documentElement.scrollWidth <= clientWidth` | SC-004 |
| on `/`, `/status`, `/tracker` at 1920 and 2560: `scrollHeight <= clientHeight` | SC-010 |

**Expected**: all green. A track-count mismatch usually means a missing `3xl`/`4xl`
breakpoint definition; a `fit`-page height failure almost always means a grid child lost its
`min-h-0` (research R6).

## Step 4 — Contrast and accessibility

```bash
pnpm --filter @job-finder/dashboard test:e2e -- contrast
```

Runs axe with the `color-contrast` rule over all nine routes in **both** appearances
(`data-theme="dark"` and `="light"`).

**Expected**: zero violations → SC-006. Keyboard reachability (SC-009) is asserted in the same
spec by tabbing through the shell and confirming a visible focus ring on every nav
destination and primary control.

## Step 5 — Existing behaviour suites

```bash
pnpm --filter @job-finder/dashboard test:e2e
```

`feed.spec.ts`, `navigation.spec.ts`, `sources.spec.ts` must still pass. Selector-only edits
are acceptable; a changed assertion about *behaviour* means something regressed (FR-016).

## Step 6 — Manual visual pass

At 2560×1440, dark appearance, walk all nine pages and confirm:

1. 5 tile columns, content centred within the 140rem cap, tiles not stretched (User Story 1).
2. Feed, status and tracker fill the viewport with **no page scroll**; their long lists scroll
   inside their tiles, and each scroll region is reachable with Tab (FR-020, FR-021).
3. Job detail, profile, tailor, settings, sources, contacts scroll the page normally and
   nothing is clipped.
4. Greyscale everywhere except: primary buttons, active nav, focus rings, selection, and
   single-series data emphasis (FR-006). No gradient washes.
5. Force a widget into each state (stop the API for error, use an empty account for empty) and
   confirm the tile keeps its footprint in all three (FR-009).
6. Toggle to light appearance — identical layout, inverted neutrals (FR-008).
7. Resize to 375px — single column, no horizontal scroll, all nav reachable.

## Step 7 — Cross-suite gate

```bash
make test-lint
```

Required before merge (constitution IV). Then re-run the Step 0 build and compare the bundle
delta against the recorded baseline.

## Definition of done

All nine pages converted, Steps 1–7 green, bundle within budget, and SC-001 through SC-010
demonstrably asserted. Nothing merges to `master` until every step passes — the rollout is
big-bang by decision (clarification Q2).
