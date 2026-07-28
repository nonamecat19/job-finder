# Data Model: Global Dashboard Grid Layout

**Feature**: Global Dashboard Grid Layout  
**Date**: 2026-07-28  
**Note**: This feature is purely presentational. There are no persistent data entities, database tables, or API resources. The "data model" below documents the layout domain entities — the semantic objects that define how information is arranged on screen.

---

## Layout Entities

### Entity: GridContainer

The top-level responsive grid wrapper that establishes columns, gaps, and alignment for a dashboard page's main content area.

| Attribute | Type | Description |
|-----------|------|-------------|
| `columns` | Responsive enum | Desktop: 3, Tablet: 2, Mobile: 1 |
| `gap` | Responsive size | Desktop: `gap-5` (1.25rem), Mobile: `gap-3` (0.75rem) |
| `alignment` | Enum | `items-start` (cards keep intrinsic height) |
| `maxWidth` | CSS value | Inherits from parent `<main>`; no additional max-width constraint |

**Constraints**:
- Must not overlap or be pushed off-screen by the sticky sidebar (`app/shell.tsx`)
- Must not cause horizontal scroll at any viewport width

---

### Entity: GridCard

A grid item representing a single information panel or card. Wraps the existing `Surface` component and controls how many columns it occupies.

| Attribute | Type | Description |
|-----------|------|-------------|
| `span` | Semantic enum | `narrow` (1 col), `wide` (2 col), `full` (3 col) |
| `spanTablet` | Semantic enum | Derived from `span`: `narrow`→1, `wide`/`full`→full (2 col) |
| `spanMobile` | Semantic enum | Always 1 column (full width) |
| `children` | ReactNode | The card content (typically a `Surface` with internal layout) |

**Span Rules (semantic mapping)**:

| Card Content Type | Recommended `span` | Rationale |
|-------------------|-------------------|-----------|
| Analytical panel (fit summary, ghost signal, keyword diff) | `wide` | Dense content, charts, tables — needs horizontal space |
| Utility card (contacts, company intel, referrals) | `narrow` | Compact actions, short lists — fits in 1 column |
| Coach + Outreach stack | `narrow` | Two related cards stacked vertically in 1 column |
| Prep pack, Documents | `wide` or `full` | Long-form content — needs width |
| Full-page lists (feed, tracker when virtualized) | `full` | Virtualized lists must span full available width |

**Constraints**:
- Internal content must not overflow the grid cell
- Must maintain existing `Surface` padding and styling
- Must not alter its own DOM element type (remains `<section>` via `Surface`)

---

### Entity: PageLayout

The composite layout for a single dashboard page, consisting of one `GridContainer` containing multiple `GridCard`s.

| Attribute | Type | Description |
|-----------|------|-------------|
| `page` | Enum | `job-detail`, `feed`, `tracker`, `contacts`, `sources`, `profile`, `settings`, `status`, `tailor` |
| `grid` | GridContainer | The container for this page |
| `cards` | Array<GridCard> | Ordered list of cards in DOM order (source order = reading order) |

**Page-Specific Span Assignments**:

#### Job Detail Page (`features/job-detail/JobDetailPage.tsx`)

| Row | Card | Span |
|-----|------|------|
| 1 | FitSummary | `full` (3 col) |
| 2 | GhostSignalPanel | `wide` (2 col) |
| 2 | PostAgeSignal (right sidebar) | `narrow` (1 col) |
| 3 | ContactLine | `narrow` (1 col) |
| 3 | CompanyIntelCard | `narrow` (1 col) |
| 3 | ReferralPathsCard | `narrow` (1 col) |
| 4 | KeywordDiffPanel | `wide` (2 col) |
| 4 | CoachPanel + OutreachPanel (stacked) | `narrow` (1 col) |
| 5 | PrepPackPanel | `full` (3 col) |
| 6 | DocumentsPanel | `full` (3 col) |
| 7 | Job description Surface | `wide` (2 col) |
| 7 | ResumePreview (if exists) | `narrow` (1 col) |

#### Feed Page (`features/feed/FeedPage.tsx`)

| Card | Span |
|------|------|
| Filter bar / summary cards | `narrow` or `wide` depending on content |
| Job list (VirtualList) | `full` (3 col) — list stays full width, individual job cards remain stacked vertically inside the virtualized list |

*Note*: The feed page may choose to place summary/stat cards alongside the list in a 2-column layout (list = `wide`, summary = `narrow`) if virtualizer compatibility is verified.

#### Tracker Page (`features/tracker/TrackerPage.tsx`)

| Card | Span |
|------|------|
| Application cards grid | `narrow` (1 col each) in a 3-column grid, or `wide` (2 col) for detail-heavy cards |

#### Other Pages (contacts, sources, profile, settings, status, tailor)

| Card | Span |
|------|------|
| All content cards | Determined per-page; default to `narrow` for forms/settings, `wide` for tables/lists |

---

## Validation Rules

1. **No overlap**: A `GridCard` with `span=wide` must not be placed in the third column of a 3-column grid (it would overflow). The page component author is responsible for correct placement order.
2. **Source order = visual order**: Cards must be placed in DOM order such that the visual left-to-right, top-to-bottom reading order matches the source order for screen reader compatibility.
3. **Responsive degradation**: Every `GridCard` must degrade predictably: desktop 3-col → tablet 2-col → mobile 1-col without content loss.
