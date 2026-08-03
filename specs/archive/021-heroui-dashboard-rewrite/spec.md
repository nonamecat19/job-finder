> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/profile-and-dashboard.md`](../../domains/profile-and-dashboard.md) — read that first.

---
# Feature Specification: HeroUI Tile-Grid Dashboard Rewrite

**Feature Branch**: `021-heroui-dashboard-rewrite`

**Created**: 2026-07-28

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "rewrite all dashboard completely to hero ui. all design should be create like tiles/widget grid, on 32inch monitor should be 4-5 columns (its ok if big tile can be more than 1column or 1 row). All design should be modern. Theme will (black, white and accent color if needed). References: 4 monochrome bento-grid dashboard reference images"

## Clarifications

### Session 2026-07-28

- Q: How deep does the HeroUI adoption go — full replacement of the existing component layer, hybrid, or tokens only? → A: Full swap — HeroUI replaces all Radix primitives and the bespoke `ui.tsx` layer; Radix dependencies are removed. Drag-and-drop (dnd-kit) and list virtualization (react-virtual) are retained, as HeroUI has no equivalent.
- Q: Does the dashboard fit the viewport with tiles scrolling internally, or does the page scroll? → A: Hybrid — overview pages (feed, status, tracker) fit the viewport at ≥1024px with fixed-height tiles that scroll their own content; content-heavy pages (job detail, profile, tailor, settings, sources, contacts) scroll the page normally. Below 1024px all pages scroll the page.
- Q: Is there a chromatic accent colour, or is the design pure monochrome? → A: One chromatic accent, used sparingly — a single hue applied to primary action, active navigation, focus ring, selection, and single-series data emphasis. The exact hue is chosen during design; the palette is otherwise greyscale.
- Q: How are the layout success criteria (column counts, no horizontal scroll, contrast) verified? → A: Automated browser tests across a fixed viewport matrix (375, 768, 1280, 1920, 2560, 3840 px) assert column counts, absence of horizontal scroll, and contrast on every page; component-level tests keep covering behaviour and widget loading/empty/error states. No screenshot baselines.
- Q: How does the rewrite roll out — big bang, foundation-first incremental, or behind a theme flag? → A: Big bang — foundation, all nine converted pages, and removal of the replaced component layer land together as a single deliverable; no intermediate state where old and new designs coexist in a shipped build.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Ultra-Wide Tile Grid (Priority: P1)

As a job seeker working on a large (32-inch, ~2560px+) monitor, I want every dashboard page laid out as a dense grid of tiles/widgets that expands to 4–5 columns, so that the screen is fully used and I can see most of a page's information without scrolling.

**Why this priority**: The stated core problem is wasted horizontal space — today's layout caps at 3 columns and leaves large monitors half empty. Column density is the single change that delivers most of the perceived value, and it can ship before any visual restyle.

**Independent Test**: Open any dashboard page at 2560px width and count rendered columns in the main content area; verify 4 or 5 tile columns are occupied, tiles are aligned to a shared grid, and no tile is stretched to an unreadable width.

**Acceptance Scenarios**:

1. **Given** a viewport of 2560px or wider, **When** any dashboard page loads, **Then** the main content area renders tiles across 5 grid columns.
2. **Given** a viewport between 1920px and 2559px, **When** any dashboard page loads, **Then** the main content area renders tiles across 4 grid columns.
3. **Given** a tile designated as "feature" (e.g., a primary chart, a job description, a long list), **When** the page renders on a ≥1920px viewport, **Then** that tile spans 2 or more columns and/or 2 or more rows while remaining aligned to the same grid tracks as single-cell tiles.
4. **Given** any viewport width from 320px to 3840px, **When** any dashboard page renders, **Then** there is no horizontal page scroll and no tile overflows its cell.

---

### User Story 2 - Monochrome Modern Visual System (Priority: P1)

As a user, I want the whole dashboard to use a single modern black-and-white visual language — high-contrast surfaces, generous rounding, restrained typography, and one accent colour used sparingly — so the product feels like a deliberate, contemporary operations desk rather than a default component kit.

**Why this priority**: The requested redesign is as much about visual identity as density; without a unified monochrome token set, converted pages would look inconsistent with unconverted ones during rollout.

**Independent Test**: Audit every dashboard page against the token set: confirm all backgrounds, surfaces, borders and text resolve to neutral (black/white/grey) tokens, and that the accent token appears only in the permitted roles.

**Acceptance Scenarios**:

1. **Given** the dashboard in its default theme, **When** any page is rendered, **Then** all surface, border, and text colours resolve to neutral greyscale tokens (no multi-hue brand colours or decorative gradients remain).
2. **Given** any page, **When** the accent colour is used, **Then** it appears only for interactive emphasis (primary action, active navigation item, focus ring, selected state) or single-series data emphasis — never as a decorative background wash.
3. **Given** semantic states (success, warning, danger), **When** they are displayed, **Then** they remain visually distinguishable from the neutral palette while staying subdued enough not to compete with the accent.
4. **Given** the same page in dark and light appearance, **When** compared, **Then** both use identical layout and spacing and differ only by inverted neutral tokens.
5. **Given** any text on any tile, **When** contrast is measured, **Then** body and label text meets WCAG AA contrast (4.5:1) against its tile background.

---

### User Story 3 - Every Page Rebuilt as Widgets (Priority: P1)

As a user navigating between feed, job detail, tracker, tailor, contacts, sources, status, profile and settings, I want each page composed of self-contained widget tiles with a consistent header/body/footer rhythm, so that every screen is scannable in the same way and nothing is a full-width wall of form fields.

**Why this priority**: A grid container alone does not deliver the reference look; the content itself must be decomposed into widgets. This is the bulk of the work and defines "rewrite completely".

**Independent Test**: Visit each of the nine pages and confirm every block of content lives inside a tile with consistent tile chrome (title, optional action affordance, body), with no legacy full-bleed sections remaining.

**Acceptance Scenarios**:

1. **Given** any of the nine dashboard pages, **When** it renders, **Then** every content block is contained in a tile using the shared tile chrome, and no page uses a bespoke one-off card style.
2. **Given** a page with headline metrics (status, tracker, feed), **When** it renders, **Then** metrics are presented as compact single-cell KPI tiles showing a value, a label, and a trend/context line.
3. **Given** a widget whose data is unavailable, loading, or empty, **When** the page renders, **Then** the tile still occupies its grid cell and shows a skeleton (loading) or an explicit empty-state message (no data) rather than collapsing.
4. **Given** a widget whose content is longer than its allotted height, **When** it renders, **Then** the content scrolls inside the tile or is truncated with an affordance to see more, without changing the tile's grid footprint.

---

### User Story 4 - Restyled App Shell and Navigation (Priority: P2)

As a user, I want the sidebar, top bar and page headers to match the new monochrome tile language — a compact icon-led rail, clear active state, and page identity that does not steal vertical space from the grid.

**Why this priority**: The shell frames every page; leaving it in the old style would undermine the redesign, but pages can be converted and demoed before the shell changes.

**Independent Test**: Load any page and verify the shell chrome uses the new tokens, the active nav item is unambiguous, and the shell consumes no more vertical space above the grid than it does today.

**Acceptance Scenarios**:

1. **Given** a desktop viewport, **When** the shell renders, **Then** the sidebar uses neutral surfaces with the accent reserved for the active item, and remains fixed while main content scrolls.
2. **Given** a mobile viewport (<768px), **When** the shell renders, **Then** navigation remains fully reachable and every nav destination has exactly one accessible interactive element in the DOM.
3. **Given** any page, **When** it renders, **Then** the page title/description area occupies at most one grid row's worth of vertical space before the first tile.

---

### User Story 5 - Responsive Down-Scaling (Priority: P2)

As a user on a laptop, tablet or phone, I want the same tiles to reflow to fewer columns without losing content or becoming cramped.

**Why this priority**: The 4–5 column target is the headline, but the dashboard must stay usable on smaller screens; regressions here would break daily use.

**Independent Test**: Sweep the viewport from 320px to 3840px and verify column counts change only at defined breakpoints, with no clipped content, overlap, or horizontal scroll at any width.

**Acceptance Scenarios**:

1. **Given** a viewport below 640px, **When** any page renders, **Then** all tiles are single-column, full-width, with safe edge padding.
2. **Given** a viewport between 640px and 1023px, **When** any page renders, **Then** tiles render in 2 columns and multi-column tiles fall back to full width.
3. **Given** a viewport between 1024px and 1919px, **When** any page renders, **Then** tiles render in 3 columns.
4. **Given** any breakpoint change, **When** tiles reflow, **Then** no tile's content is clipped and no interactive control becomes unreachable.

---

### Edge Cases

- A page has fewer tiles than available columns (e.g., settings on a 5-column grid): remaining tracks stay empty and tiles keep a sane maximum width rather than stretching into unreadable line lengths.
- A tile's content is far taller than its neighbours (e.g., a full job description): the tile spans extra rows and/or scrolls internally; neighbouring tiles are not force-stretched to match height.
- An overview page holds more content than one viewport can fit even after in-tile scrolling (e.g., a very long tracker): the page must still not introduce page-level scroll at ≥1024px — tiles absorb the overflow internally, and any tile that cannot do so is redesigned or relocated rather than allowed to push the grid past the viewport.
- A viewport is short (e.g., 2560×1080 or a half-height window): fit-to-viewport overview pages must keep tiles above a minimum readable height, falling back to page scroll rather than crushing tiles below it.
- Virtualized lists (feed results) placed inside a tile: the tile must give the virtualizer a bounded, measurable height so row virtualization keeps working.
- Ultra-wide monitors beyond 3840px: column count is capped at 5 and the grid is centred with a maximum content width so tiles never become absurdly wide.
- A widget errors while fetching: the tile shows an inline error state with a retry affordance and keeps its grid footprint; a single failing widget never blanks the page.
- Dense text tiles (job description, generated resume) must retain a readable line length even when spanning multiple columns.
- Reduced-motion preference: tile hover/entry transitions are suppressed when the user requests reduced motion.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The dashboard MUST render all page content as tiles on a shared responsive grid with these column counts: 1 column below 640px, 2 columns at 640–1023px, 3 columns at 1024–1919px, 4 columns at 1920–2559px, and 5 columns at 2560px and above.
- **FR-002**: The grid MUST support tiles that span multiple columns and/or multiple rows, with span rules that degrade predictably at each smaller breakpoint (a span never exceeds the available column count).
- **FR-003**: Grid gaps, tile corner radius, tile padding, and tile border treatment MUST be defined once as shared tokens/primitives and used by every page; per-page bespoke card styling is not permitted.
- **FR-004**: All nine dashboard pages (feed, job detail, tracker, tailor, contacts, sources, status, profile, settings) MUST be rebuilt on the tile grid, with all existing functionality preserved and reachable.
- **FR-005**: The colour system MUST be monochrome-first: background, surface, elevated surface, border, and text tokens MUST all be neutral greyscale, plus exactly one chromatic accent token (a single hue, not a greyscale value) and the semantic status tokens.
- **FR-006**: The accent colour MUST be restricted to interactive emphasis (primary buttons, active nav, focus rings, selection) and single-series data emphasis; decorative gradients and multi-hue brand washes MUST be removed.
- **FR-007**: Semantic status colours (success, warning, danger) MUST remain available and distinguishable, expressed in a subdued form consistent with the monochrome system.
- **FR-008**: Both dark and light appearances MUST be supported, sharing identical layout and differing only in neutral token values.
- **FR-009**: Every tile MUST define loading (skeleton), empty, and error presentations that preserve the tile's grid footprint.
- **FR-010**: The dashboard MUST NOT produce horizontal page scroll at any viewport width between 320px and 3840px.
- **FR-011**: At widths above the 5-column breakpoint, the grid MUST cap total content width and centre it so tiles do not exceed a readable maximum width.
- **FR-012**: The app shell (sidebar navigation, mobile navigation, notification affordance, page header) MUST adopt the new tile/monochrome language while preserving all existing navigation destinations and the existing behaviour of exactly one accessible element per nav destination.
- **FR-013**: Interactive elements MUST expose a visible focus indicator using the accent token, and all tiles and controls MUST remain keyboard-navigable in a logical order.
- **FR-014**: Text and essential UI elements MUST meet WCAG AA contrast against their tile surface in both appearances.
- **FR-015**: Motion (tile hover, entry, state transitions) MUST be subtle and MUST be disabled when the user prefers reduced motion.
- **FR-016**: The redesign MUST be presentation-layer only: no changes to API contracts, data fetching semantics, or business logic; existing behaviour tests MUST continue to pass or be updated only for markup/selector changes.
- **FR-017**: Virtualized list surfaces MUST continue to virtualize after conversion, with the containing tile providing a bounded height.
- **FR-020**: The dashboard MUST use a hybrid scroll model at viewports ≥1024px: overview pages (feed, status, tracker) MUST fit within the viewport with no page-level vertical scroll, their tiles taking fixed heights and scrolling their own content; content-heavy pages (job detail, profile, tailor, settings, sources, contacts) MUST scroll at the page level with tiles sized to their content. Below 1024px every page scrolls at the page level and no tile is height-constrained.
- **FR-021**: Any tile that scrolls internally MUST keep its scrollable region reachable by keyboard and MUST indicate that more content exists beyond the visible area.
- **FR-018**: A developer MUST be able to add a new page or widget using the shared grid and tile primitives with no page-specific layout CSS.
- **FR-019**: The dashboard MUST end up with a single component foundation: no two overlay/focus-management systems may coexist after the rewrite, and every interaction behaviour provided today by the replaced primitives (modal focus trapping and dismissal, keyboard-operable select, switch, tabs, toast announcements, tooltips) MUST be preserved by the replacement components.

### Key Entities

- **Grid Container**: Page-level layout primitive defining the responsive column tracks (1/2/3/4/5), gaps, alignment, and maximum content width.
- **Tile (Widget)**: The universal content unit — consistent chrome (title, optional action, body, optional footer), a declared column/row span, and defined loading/empty/error states.
- **Span Rule**: Semantic classification of a tile (compact KPI, standard, wide, tall, feature) mapped to concrete column/row spans per breakpoint.
- **Theme Token Set**: The named neutral scale, single accent, semantic status colours, radii, spacing, and typography scale shared by every tile and shell element.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a 2560px-wide viewport, every dashboard page fills 5 grid columns, asserted by an automated browser test running the viewport matrix (375, 768, 1280, 1920, 2560, 3840 px) against all nine pages, with the expected column count checked at each width.
- **SC-002**: On a 2560px-wide viewport, each page shows at least 60% more content tiles above the fold than the pre-rewrite layout.
- **SC-003**: All nine dashboard pages render 100% of their content inside shared tile primitives, with zero page-specific layout CSS remaining.
- **SC-004**: Zero horizontal scroll and zero clipped or overlapping content on all nine pages at every width in the automated viewport matrix, plus a 320px minimum-width check.
- **SC-005**: 100% of colour values used in the UI resolve to the shared token set; no hard-coded colours remain outside the token definitions.
- **SC-006**: 100% of text/background pairs sampled across pages meet WCAG AA contrast in both dark and light appearance, checked by an automated accessibility assertion in the same browser test run.
- **SC-007**: Every tile has a defined loading, empty, and error presentation, verified by component-level state tests for each widget.
- **SC-008**: All existing dashboard tests pass after the rewrite, with no loss of any user-facing capability that existed before it.
- **SC-009**: Every navigation destination and interactive control remains reachable by keyboard with a visible focus indicator.
- **SC-010**: At 1920px and 2560px, the three overview pages (feed, status, tracker) show zero page-level vertical scroll, and every piece of their content remains reachable through in-tile scrolling — asserted in the same automated viewport matrix.

## Assumptions

- "HeroUI" is taken to mean adopting the HeroUI React component library as the dashboard's sole component foundation. Every existing Radix primitive (dialog, select, switch, tabs, toast, tooltip) and the bespoke shared component layer are replaced by HeroUI equivalents, and the Radix dependencies are removed — no hybrid component stack remains. Drag-and-drop and list virtualization keep their current libraries, since HeroUI provides no equivalent. This is an implementation choice recorded here because the user named it explicitly; feasibility is confirmed during planning.
- The rewrite is presentation-layer only; API routes, query hooks, and data shapes are untouched.
- "32-inch monitor" is interpreted as a ~2560px-wide (QHD) viewport; 4K and ultra-wide widths reuse the same 5-column maximum inside a centred, width-capped container.
- The reference images set direction (monochrome tiles, large KPI numerals, mixed light/dark tiles, generous rounding, restrained type, sparse accent), not literal content to reproduce; job-finder's own content and information architecture are preserved.
- Dark appearance remains the default, matching current behaviour, and the existing light appearance is retained and restyled rather than dropped.
- The existing page set (nine pages) and navigation structure are unchanged; no pages are added or removed.
- Existing semantic token names in the stylesheet are re-valued to greyscale rather than renamed, keeping the token vocabulary stable for downstream work.
- The hybrid scroll model classifies pages as "overview" (feed, status, tracker) or "content-heavy" (job detail, profile, tailor, settings, sources, contacts); this classification is fixed for this feature and is not user-configurable.
- Layout, overflow, and contrast criteria are verified in a real browser (the existing end-to-end test harness) because a simulated DOM cannot compute grid tracks or layout boxes; widget behaviour and state coverage stays in the existing component test suite. Screenshot/visual-regression baselines are explicitly out of scope.
- Tile layouts are author-defined per page; end users cannot rearrange, resize, or persist tile positions in this feature.
- The accent is one chromatic hue (not a greyscale emphasis), chosen during design and applied only in the roles listed in FR-006; per-user accent selection is out of scope. Emphasis techniques from the references (inverted dark tiles on a light field, weight and scale contrast) are used alongside the accent, not instead of it.
- Rollout is big-bang: the token foundation, shared primitives, all nine converted pages, and removal of the replaced component layer ship as one deliverable. No shipped build mixes old and new designs, and no feature flag or theme toggle gates the rewrite. Work may still be sequenced internally, but the feature is not complete or releasable until all nine pages are converted.
