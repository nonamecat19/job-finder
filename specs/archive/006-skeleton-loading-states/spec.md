> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/profile-and-dashboard.md`](../../domains/profile-and-dashboard.md) — read that first.

---
# Feature Specification: Skeleton Loading States

**Feature Branch**: `006-skeleton-loading-states`

**Created**: 2026-07-24

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "lets use skeletons way for all loadings in project"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See content shape while a page loads (Priority: P1)

Job seeker opens feed, tracker, status, sources, contacts, profile, settings, or job-detail pages. While data is being fetched, dashboard shows a skeleton placeholder that mirrors layout of the content about to appear (cards, rows, list items), instead of a spinner or blank area.

**Why this priority**: Highest-traffic pages (feed, job detail, tracker) currently show a bare spinner or blank space during load, which reads as slower and gives no sense of incoming layout. This is the core, visible value of the feature.

**Independent Test**: Throttle/delay any one data fetch (e.g. feed job list) and confirm a layout-matching skeleton renders during the loading window, then is replaced by real content.

**Acceptance Scenarios**:

1. **Given** feed page is opened and job list request is in flight, **When** page renders, **Then** user sees skeleton job cards in place of the eventual job list (not a spinner, not blank).
2. **Given** loading finishes and data returns, **When** response arrives, **Then** skeleton is replaced by real content with no layout jump (skeleton dimensions match real content dimensions).
3. **Given** a page section reloads (e.g. filter change re-fetches list), **When** the re-fetch is in flight, **Then** the same skeleton pattern reappears for that section only, without remounting unrelated parts of the page.

---

### User Story 2 - Consistent skeleton pattern across all loading surfaces (Priority: P2)

Developer/user encounters loading states across every feature area of the dashboard (feed, tracker, status/activity, sources, contacts, profile, settings panels, job-detail sub-panels like coach/outreach/referrals/company-intel/ghost-signal/keyword-diff/prep-pack). All of these use one shared skeleton visual language (same shimmer/pulse animation, same corner radius and spacing conventions) rather than a mix of spinners, "Loading…" text, and ad hoc placeholders.

**Why this priority**: The codebase currently mixes a generic `Spinner` component, inline "Loading…" text, and no placeholder at all across ~25 files. Standardizing prevents visual inconsistency and reduces future rework, but depends on User Story 1 establishing the pattern first.

**Independent Test**: Inventory every feature page/panel that currently shows a loading indicator; confirm each one now renders a skeleton built from the shared skeleton primitive rather than `Spinner` or raw loading text.

**Acceptance Scenarios**:

1. **Given** any dashboard page or panel that fetches data, **When** it is in a loading state, **Then** it renders a skeleton from the shared skeleton component, not the old `Spinner` component or inline loading text.
2. **Given** two different pages with structurally different content (e.g. a list of rows vs. a detail card), **When** each is loading, **Then** each skeleton's shape reflects that page's own layout, while sharing the same animation/style tokens.

---

### User Story 3 - Skeletons respect fast responses and errors (Priority: P3)

User on a fast connection or cached data barely sees the loading state at all; user on an errored request sees the existing error state, not a stuck skeleton.

**Why this priority**: Polish/edge-case correctness — prevents skeleton flicker on fast loads and ensures skeletons don't mask or delay error handling.

**Independent Test**: Simulate a near-instant response and confirm no jarring skeleton flash; simulate a failed request and confirm error state displays instead of skeleton.

**Acceptance Scenarios**:

1. **Given** a request resolves in under a short threshold, **When** the response returns, **Then** skeleton either does not flash noticeably or is shown for a minimum consistent duration to avoid flicker.
2. **Given** a request fails, **When** the error is known, **Then** the existing `ErrorState` is shown and skeleton is removed, never shown simultaneously with or after the error.

---

### Edge Cases

- What happens when a list is loading but was previously populated (e.g. filter change on tracker/feed)? Skeleton should replace only the previously-loaded content region, not the whole page chrome (filters, headers stay visible).
- How does system handle a page with zero expected items once loaded (skeleton for a list that turns out empty)? Skeleton disappears and existing `EmptyState` shows, per current behavior.
- How does system handle nested/partial loading (e.g. job list loaded but one job-detail sub-panel like `CoachPanel` still loading)? Only the still-loading sub-panel shows its own skeleton; already-loaded panels are unaffected.
- What happens on very small viewport widths (mobile-ish dashboard width)? Skeleton layout must adapt/reflow the same way the real content does.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide one shared skeleton UI primitive (or small family of primitives: text line, block/card, avatar/circle) usable across all dashboard features.
- **FR-002**: Every dashboard page and panel that currently shows a loading indicator (`Spinner` usage or inline "Loading…" text) MUST be migrated to render a skeleton built from the shared primitive instead.
- **FR-003**: Each skeleton MUST approximate the layout (count, size, arrangement) of the content it precedes, so replacing skeleton with real content causes no visible layout shift.
- **FR-004**: Skeleton MUST be removed and replaced by real content as soon as data arrives, and MUST be removed in favor of `ErrorState` when the request fails — skeleton and error/content MUST NOT render simultaneously.
- **FR-005**: Skeleton animation/style (shimmer or pulse) MUST be visually consistent (same timing, same base styling tokens) across every feature area.
- **FR-006**: Loading state changes MUST be scoped to the specific section being reloaded (e.g. list re-fetch on filter change) without remounting or re-skeletonizing unrelated page regions.
- **FR-007**: Skeletons MUST remain accessible — they must not be announced by screen readers as real content, and loading status must still be conveyed to assistive technology (e.g. via an appropriate live-region or busy state).

### Key Entities

- **Skeleton primitive**: Visual placeholder component(s) representing loading content shape (line, block, circle); no persisted data, purely presentational.
- **Loading state**: Per-page/per-panel boolean/derived status (already exists via `isLoading`/`isPending` data-fetch flags) that now drives skeleton rendering instead of spinner rendering.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of dashboard pages/panels that previously used `Spinner` or inline "Loading…" text now use the shared skeleton primitive.
- **SC-002**: Zero visible layout shift (content jump) measured between skeleton and loaded state on the top 3 highest-traffic pages (feed, tracker, job detail).
- **SC-003**: Users perceive page transitions as at least as fast or faster than before, with no reported increase in "loading felt slow/broken" feedback after rollout.
- **SC-004**: No instance of skeleton and error state (or skeleton and stale error) rendering at the same time across any tested page.

## Assumptions

- "Skeletons way" means replacing the existing `Spinner` component and any inline loading text with shimmer/pulse-style content-shaped placeholders, applied consistently dashboard-wide — not a partial or single-page change.
- Existing `EmptyState` and `ErrorState` components remain unchanged and continue to be used for their respective cases; skeleton only owns the in-flight loading case.
- Scope is the `apps/dashboard` React app; no changes to API response shape or timing are required.
- Minimum-display-duration / anti-flicker behavior (User Story 3) is a nice-to-have refinement, not a hard blocker for initial rollout.
- Accessibility handling reuses standard patterns (e.g. `aria-busy`, `role="status"`) rather than introducing a new a11y framework.
