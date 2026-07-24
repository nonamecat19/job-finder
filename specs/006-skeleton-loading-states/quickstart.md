# Quickstart: Validate Skeleton Loading States

No `contracts/` directory: this feature exposes no new API/CLI/external interface — it's
an internal UI presentational change within `apps/dashboard`. Validation is manual/visual
plus the existing `vitest` suite.

## Prerequisites

- `pnpm install` at repo root (workspace deps installed)
- `pnpm --filter @job-finder/shared build` (per Constitution workflow, shared types built
  first — not touched by this feature, but keeps other dashboard tooling consistent)
- Dashboard dev server runnable via `apps/dashboard`: `pnpm --filter @job-finder/dashboard dev`
  (or repo's `make dev` / `make up` per Constitution workflow conventions)

## Run

1. Start the stack (`make up` or `make dev` per repo convention) so the API backing the
   dashboard is reachable.
2. Start the dashboard dev server if not already part of `make dev`.
3. Open the dashboard in a browser.

## Validation scenarios (map to spec.md Acceptance Scenarios)

1. **Feed skeleton (US1, AS1/AS2)**: Throttle network (browser devtools → Network →
   Slow 3G) and reload the Feed page. Confirm skeleton job cards render matching the
   real card grid shape, then are replaced by real cards with no layout jump once data
   arrives.
2. **Scoped re-fetch skeleton (US1, AS3)**: On Feed or Tracker, change a filter that
   triggers a re-fetch. Confirm only the list region re-shows its skeleton; header/filter
   controls remain visible and do not remount.
3. **Cross-page consistency (US2)**: Visit Feed, Tracker, Status, Sources, Contacts,
   Profile, Settings, and a Job Detail page with at least one still-loading sub-panel
   (e.g. CoachPanel). Confirm each uses the shared skeleton primitives (shimmer/pulse
   look and timing match) rather than the old `Spinner`/loading text.
4. **Nested partial loading (Edge Case)**: Open a Job Detail page where the main job data
   has already loaded but `CoachPanel` or `OutreachPanel` is still fetching. Confirm only
   that panel shows a skeleton; the rest of the page is unaffected.
5. **Error precedence (US3, AS2)**: Force a request to fail (e.g. stop the API mid-load
   or use devtools request blocking). Confirm the skeleton is removed and `ErrorState`
   renders — never both at once.
6. **Empty state precedence (Edge Case)**: Load a page/filter combination known to return
   zero results. Confirm skeleton is replaced by the existing `EmptyState`, not an
   empty skeleton or blank region.

## Automated checks

- `pnpm --filter @job-finder/dashboard test` (vitest) — run after migrating each
  page/panel; update or add tests asserting skeleton markup (e.g. `role="status"`,
  `aria-busy="true"`) replaces prior `Spinner`/text assertions.
- `pnpm --filter @job-finder/dashboard typecheck` — ensure new primitives are fully typed.
- Per Constitution IV, since this change is scoped to a single app (`apps/dashboard`), the
  dashboard's own `vitest` suite passing is the required gate; no
  `make test-integration`/`make test-e2e` requirement is triggered unless a Playwright
  visual-regression case is added for skeleton→content transitions.
