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
