# design-sync notes — @job-finder/dashboard

## Shape of this repo

- This is **not** a design-system package. `apps/dashboard` is a private Vite app with
  no library build, so there is no `dist/` entry to bundle. `.design-sync/entry.ts` is a
  hand-written barrel that serves as the synthetic package entry — pass it with
  `--entry ./.design-sync/entry.ts`. Without `--entry` the converter looks for
  `node_modules/@job-finder/dashboard/package.json` and dies with ENOENT.
- Add a component to the kit by exporting it from `.design-sync/entry.ts` **and** adding
  it to `cfg.componentSrcMap` (the map is the component list here; there are no `.d.ts`
  exports to discover from).
- `--node-modules ./node_modules` (the dashboard's own) resolves `react`/`react-dom` fine
  under pnpm; the repo root has no `node_modules/@heroui`.

## CSS pipeline

- Tailwind v4 is wired through `@tailwindcss/vite`, not a CLI. `src/index.css` does
  `@import '@heroui/styles'`, which is a **transitive** pnpm dep — resolvable by Vite but
  **not** by the standalone `@tailwindcss/cli`. Don't try to compile the stylesheet
  outside the app build; it will fail with `Can't resolve '@heroui/styles'`.
- So `cfg.buildCmd` runs the app's own `pnpm build` and copies `dist/assets/index-*.css`
  to `.design-sync/.cache/tailwind.css` (`cfg.cssEntry`), rewriting the absolute
  `url(/fonts/…)` font references to bare filenames so the converter's `fonts/` rewrite
  resolves. Keep that `sed` in sync if the font paths ever change.
- **`src/index.css` carries two `@source` lines** (added by design-sync) pointing at
  `.design-sync/previews` and `.design-sync/root.tsx`. Tailwind's automatic source
  detection skips dot-directories, so without them any utility used only in a preview is
  silently absent from the compiled CSS and the card renders unstyled. Don't remove them.
- Consequence for authoring: the shipped CSS is **static and app-derived**. A utility the
  app (or a preview) never uses is not emitted. Verify any new class against
  `ds-bundle/_ds_bundle.css` before relying on it. `bg-background-tertiary` and
  `border-separator` are examples — the tokens exist, the classes don't.

## ThemeRoot

- The preview card shell hardcodes `background:#fff`, and this DS is dark-first, so every
  card rendered dark-on-white until `ThemeRoot` was added. It is a **real component**
  (`.design-sync/root.tsx`, exported from the barrel, shipped in the bundle), wired as
  `cfg.provider`, and it is genuinely what a consumer needs — not a preview hack.
- `cfg.provider.props.style` is preview-only presentation (`padding`, `borderRadius`).
  Do **not** put `margin:-24px` there to bleed the background to the card edge: it makes
  every component 48px wider than its grid cell and trips `[GRID_OVERFLOW]` on all 22.

## Known render warns (expected — not new)

- `[TOKENS_MISSING]` — 8 vars (`--disclosure-panel-height`, `--trigger-anchor-point`,
  `--color-area-background`, `--color-area-thumb-color`, `--color-swatch-current`,
  `--color-field-border-invalid`, `--visual-viewport-height`, `--trigger-width`). These
  are HeroUI runtime vars set by JS on mount, never declared in a stylesheet. Expected.
- `[GRID_OVERFLOW]` on `Checkbox` — the `States` cell puts three labelled boxes in a row.
  Remedied with `cfg.overrides.Checkbox.cardMode = "column"`.
- `[DOCS_UNMAPPED]` for all components — there is no per-component docs tree in this repo,
  so every `.prompt.md` is synthesized from the `.d.ts` + preview. Fine as-is.

## Previews

- `ToastProvider` renders no chrome of its own; toasts need a runtime emit a static card
  can't trigger. Its preview shows the provider wrapping app content and explains that in
  copy. Hover/drag/focus states are likewise not covered by any preview.
- `GhostBadge` returns `null` below score 50, so its cells only use scores ≥ 50.
- `Switch`, `Tabs` and `TagInput` were added in the 2026-08 re-sync (they had drifted into
  `src/components/ui.tsx` after the first sync). All three are controlled, so their previews
  pass fixed props + a `noop` handler — the same pattern `RangeSlider.tsx` uses.
- Previews deliberately avoid `lucide-react`: preview files import from `@job-finder/dashboard`
  only, and icons are not part of the barrel. `Tabs`'s `TabItem.icon` is therefore unexercised
  by its card.
- Preview composition was ported from real usage: `TrackerPage.tsx` (EmptyState,
  LoadingRegion, Field+Select), `NormalEntryForm.tsx` (Field grid), `ContactLine.tsx`.

## Playwright

- Chromium build `1228` is cached in `~/.cache/ms-playwright/`; the matching release is
  `playwright@1.61.1`, which is what `.ds-sync/` installs. A different version fails with
  `browserType.launch: Executable doesn't exist`.

## Re-sync risks

- **`src/index.css` `@source` lines** are the fragile link. If someone reformats or prunes
  that file, previews silently lose their utilities and cards render unstyled while every
  check still passes. Diff those two lines first on any re-sync.
- **`cfg.cssEntry` is a build artifact**, regenerated by `buildCmd` into a gitignored
  cache. A fresh clone must run `buildCmd` before the converter, or the build fails on a
  missing stylesheet.
- **`.design-sync/entry.ts` and `cfg.componentSrcMap` will drift** from `src/components/`
  as the app grows — new components are invisible to the sync until both are updated.
  Check `src/components/ui.tsx` exports against the map each time.
- **`ThemeRoot` lives only in `.design-sync/`**, not in app source. If the app's own theme
  handling changes (token names, `data-theme` attribute), `root.tsx` must be updated by
  hand — nothing links them.
- **Font files** are copied from `public/fonts/` by an enumerated `cfg.extraFonts` list.
  Adding or renaming a woff2 in the app requires regenerating that list.
- **`--surface-track`** is the only token `Tabs` needs that no other component uses; it is
  emitted via the arbitrary utility `bg-[var(--surface-track)]`, so a Tailwind source-scan
  regression would drop it silently. Check it in `ds-bundle/_ds_bundle.css` on re-sync.
- Scope excluded deliberately: `src/components/layout/`, `VirtualList`, and all feature
  components. `useToast` is exported from the barrel but excluded from the component map
  (it's a hook).
