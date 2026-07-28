# Feature Specification: Global Dashboard Grid Layout

**Feature Branch**: `001-global-dashboard-grid`

**Created**: 2026-07-28

**Status**: Draft

**Input**: User description: "Implement grid layout on dashboard globally - apply a bento-box style dashboard grid layout system across all dashboard pages, similar to the CRM dashboard reference design. The layout should use a consistent grid system with cards that can span multiple columns/rows, creating a dense, information-rich dashboard experience."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Job Detail Grid Layout (Priority: P1)

As a job seeker reviewing a specific job posting, I want the analytical cards (fit score, ghost-job signal, contacts, company intel, keyword match, coach, outreach, prep pack) to be arranged in a dense bento-box grid so I can scan all insights at a glance without excessive scrolling.

**Why this priority**: The job detail page is the highest-traffic analytical surface in the app; users need to compare multiple signals quickly to decide whether to apply. The current layout has cards stacked vertically with inconsistent widths, creating unnecessary whitespace and hiding related information below the fold.

**Independent Test**: Can be fully tested by navigating to any job detail page and verifying that all analytical cards appear in a multi-column grid with no card wider than its allocated column span, and that related cards (e.g., coach + outreach) sit adjacent to each other.

**Acceptance Scenarios**:

1. **Given** a user opens any job detail page on a desktop viewport (≥1024px), **When** the page loads, **Then** the cards are arranged in a 3-column grid where wide analytical panels (fit summary, ghost signal, keyword diff, prep pack) span 2 columns, and narrow utility cards (contacts, company intel, coach, outreach) occupy 1 column.
2. **Given** a user opens a job detail page on a tablet viewport (768px–1023px), **When** the page renders, **Then** the grid collapses to 2 columns with the same semantic spanning rules applied proportionally (2-col spans become full width, 1-col cards pair side-by-side).
3. **Given** a user opens a job detail page on a mobile viewport (<768px), **When** the page renders, **Then** all cards stack in a single column with full width and no horizontal overflow.

---

### User Story 2 - Feed and Tracker Grid Layout (Priority: P1)

As a job seeker browsing my job feed or application tracker, I want job cards and application cards to be arranged in a responsive grid so I can see more listings per screen and compare opportunities visually.

**Why this priority**: The feed and tracker are the primary list-based surfaces; converting them from stacked vertical lists to a grid improves information density and makes the dashboard feel like a modern operations desk rather than a simple list.

**Independent Test**: Can be fully tested by navigating to the feed page and tracker page and verifying that cards appear in a multi-column grid on desktop/tablet, with each card maintaining its internal content structure and not overflowing its container.

**Acceptance Scenarios**:

1. **Given** a user visits the feed page on desktop, **When** the page loads, **Then** job cards are displayed in a 2-column grid with consistent vertical rhythm and equal card heights within each row.
2. **Given** a user visits the tracker page on desktop, **When** the page loads, **Then** application cards are displayed in a 2-column grid with status indicators aligned across cards.
3. **Given** a user on either feed or tracker, **When** they resize the browser to tablet width, **Then** the grid remains 2-column but adapts gutters and padding responsively.

---

### User Story 3 - Consistent Grid System Across All Pages (Priority: P2)

As a user navigating between different sections of the dashboard (contacts, sources, profile, settings, status, tailor), I want the layout rhythm to feel consistent so I can build spatial familiarity and predict where content will appear.

**Why this priority**: Spatial consistency reduces cognitive load and makes the app feel professionally designed; users should not experience a layout "jump" when switching from job detail to profile to settings.

**Independent Test**: Can be fully tested by visiting each dashboard page in sequence and verifying that the main content area uses the same grid container, same gap spacing, and same card surface styling.

**Acceptance Scenarios**:

1. **Given** a user navigates from job detail → profile → settings → sources, **When** each page loads, **Then** the main content area uses the same grid system container with identical gap spacing (16–20px), identical card border radius, and identical background/surface layering.
2. **Given** a developer adds a new dashboard page in the future, **When** they use the standard grid container component, **Then** the new page automatically inherits the global grid behavior without custom CSS.

---

### User Story 4 - Responsive Grid Adaptation (Priority: P2)

As a user on a tablet or small laptop, I want the grid to adapt intelligently to my screen size so I still get a multi-column layout where appropriate, but never have cards crushed or unreadable.

**Why this priority**: A significant portion of users may browse on tablets or split-screen laptop windows; the grid must gracefully degrade without hiding functionality.

**Independent Test**: Can be fully tested by resizing the browser across breakpoints and verifying that cards reflow smoothly without clipping content, overlapping, or triggering horizontal scroll.

**Acceptance Scenarios**:

1. **Given** a desktop viewport at 1280px wide, **When** the job detail page is viewed, **Then** the 3-column grid is active with ~1.25rem gaps and no card exceeds its allocated column span.
2. **Given** a tablet viewport at 820px wide, **When** any dashboard page is viewed, **Then** the grid switches to 2 columns with the same gap spacing, and no text or button is truncated.
3. **Given** a mobile viewport at 375px wide, **When** any dashboard page is viewed, **Then** the layout is single-column, cards touch the viewport edges with safe padding, and no horizontal scroll exists.

---

### Edge Cases

- What happens when a page has an odd number of cards in a 2-column grid? The last card should align to the left (standard CSS Grid behavior) and not stretch to full width unless explicitly configured to do so.
- How does the grid handle cards with very different intrinsic heights? Cards in the same row should have independent heights (no forced equal-height stretching) to prevent excessive whitespace in shorter cards; the grid uses `align-items: start` or equivalent.
- What happens if a card's content exceeds its column width on a narrow viewport? The card should scroll internally (if necessary) or wrap content, but never break out of the grid container and never cause horizontal page scroll.
- How does the grid interact with the existing sticky sidebar on desktop? The main content grid must respect the sidebar's width and not overlap or be pushed off-screen; the grid lives inside the main content area, not replacing the shell layout.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a reusable grid container component (or utility class set) that establishes a responsive column grid with consistent gap spacing across all dashboard pages.
- **FR-002**: Grid cards MUST support column spanning: 1-column (narrow), 2-column (wide), and 3-column (full-width) on desktop; spanning rules MUST degrade proportionally on smaller breakpoints.
- **FR-003**: The gap between grid cards MUST be consistent across all pages: 1.25rem (20px) on desktop, scaling down to 0.75rem (12px) on mobile.
- **FR-004**: The grid MUST be responsive with three defined breakpoints: single column below 768px, 2 columns between 768px and 1023px, and 3 columns at 1024px and above.
- **FR-005**: All existing dashboard pages (job detail, feed, tracker, contacts, sources, profile, settings, status, tailor) MUST adopt the global grid container for their main content cards.
- **FR-006**: Cards placed inside the grid MUST maintain their existing internal padding and styling (Surface component behavior) and MUST NOT overflow their grid cell or cause horizontal scroll.
- **FR-007**: The existing sidebar navigation and sticky positioning behavior MUST remain unchanged; the grid applies only to the main content area to the right of the sidebar.
- **FR-008**: During the layout transition, no existing functionality or content MAY be lost, hidden, or made inaccessible; all cards, buttons, and interactive elements MUST remain reachable.
- **FR-009**: The grid system MUST integrate with the existing Tailwind CSS v4 theme and use the project's semantic color tokens (surface, elevated, border, etc.) for any new container backgrounds or borders.
- **FR-010**: Virtualized lists (e.g., feed job list) MAY retain their vertical list layout internally if grid conversion would break virtualization, but the list container itself SHOULD sit within the grid if there are adjacent summary cards.

### Key Entities

- **Grid Container**: The top-level layout wrapper for each page's main content area; defines columns, gaps, and responsive breakpoints. Not a data entity but a layout primitive.
- **Grid Card / Surface**: The existing Surface component enhanced to behave as a grid item; represents a self-contained unit of information (fit score, ghost signal, contact info, etc.).
- **Column Span Rule**: A semantic mapping that determines how many columns a given card should occupy based on its content density and importance (e.g., analytical panels = wide, utility actions = narrow).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All 9 dashboard pages (feed, job detail, tracker, contacts, sources, profile, settings, status, tailor) render their main content using the global grid system, verified by visual inspection or automated layout test.
- **SC-002**: On desktop viewports (≥1024px), the job detail page displays at least 3 analytical cards in the initial viewport without scrolling, increasing information density by at least 40% compared to the pre-grid stacked layout.
- **SC-003**: Zero horizontal scroll is present on any dashboard page at any viewport width between 320px and 1920px, verified by scroll-width audit.
- **SC-004**: A new developer can add a new dashboard page using the grid container component and achieve consistent layout styling without writing custom CSS, measured by zero additional layout-related CSS required for new pages.
- **SC-005**: Users can navigate between any two dashboard pages and perceive the layout as visually consistent (same gaps, same card corners, same surface layering), measured by informal UX review or design-system compliance checklist.

## Assumptions

- The existing `Surface` card component in `components/ui.tsx` will continue to be the visual primitive for grid cards; the grid system adds layout structure around Surfaces without replacing them.
- Tailwind CSS v4's `@theme inline` and responsive prefixes (`sm:`, `md:`, `lg:`) are available and will be used for grid definitions, not custom CSS.
- The app shell (sidebar + main content split) will remain structurally unchanged; the grid lives inside the `<main>` content area and does not replace `app/shell.tsx`.
- Pages with virtualized lists (feed) may need to keep their list as a single grid item if row-level virtualization is incompatible with CSS Grid; this is acceptable.
- The current job detail page already has partial grid usage (`lg:grid-cols-3`) which will be refactored to use the global grid container for consistency.
- The reference CRM dashboard design uses a dark theme with rounded cards; our existing dark theme and Surface styling already align closely, so no color or theme changes are in scope.
- Grid implementation will not change any data fetching, API calls, or business logic; it is a pure presentation-layer change.
