# Feature Specification: Djinni Basic-Search Mode

**Feature Branch**: `015-djinni-basic-search-mode`

**Created**: 2026-07-26

**Status**: Draft

**Input**: User description: "for djinni should be created one more search mode. https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote also its should properly displayed on frontend. some searches like https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote have only one page. all query params from url should be displayed on frontend. years if its sequence should be displayed as a range"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Save a public Djinni basic-search URL as a subscription (Priority: P1)

A job seeker who already uses logged-in Djinni subscription dashboards
(`djinni.co/my/dashboard/subs/{id}/`) wants to also save a **public**
basic-search URL — the kind you reach by clicking filters on
`djinni.co/jobs/`, e.g.
`https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote`
— and have the system run it on the same schedule as any other
subscription, paging through every result page, including searches that
only have a single page of results.

**Why this priority**: This is the core of the request. Without a working
search mode that executes a basic-search URL end-to-end and paginates
correctly when there is only one page, none of the display work matters.

**Independent Test**: Save the two example URLs above as subscriptions,
trigger a manual run for each, and confirm the Node.js run returned jobs
from multiple pages while the Golang run (single page) completed
successfully without an infinite-pagination loop.

**Acceptance Scenarios**:

1. **Given** the operator pastes the Node.js basic-search URL into the
   New Subscription form, **When** they save it, **Then** it is accepted
   as a valid Djinni subscription and stored under the Djinni source,
   distinct in shape from a `subs/{id}/` dashboard URL.
2. **Given** a saved basic-search subscription exists, **When** the run
   executes it, **Then** the system issues the URL exactly as saved
   (preserving `search_type`, `primary_keyword`, `salary`, every
   `exp_level` value, and `employment`) and ingests the listings the
   page returns.
3. **Given** a basic-search URL whose results fit on a single page (e.g.
   the Golang example with salary=1500 and exp_level 1y–3y), **When**
   the run executes it, **Then** the run completes successfully after
   that single page and does not loop, retry, or report a failure.
4. **Given** a basic-search URL with multiple result pages, **When** the
   run executes it, **Then** the system walks each subsequent page until
   an empty page, the existing page cap, or a loop guard fires, and
   reuses the same login-aware fetch path already used for `subs/{id}/`
   URLs.
5. **Given** two saved basic-search subscriptions differ only in
   `exp_level` values, **When** both run, **Then** listings are
   deduplicated across them per existing feed behavior and each listing
   is attributed to Djinni.

---

### User Story 2 - See every basic-search filter on the subscription in the dashboard (Priority: P2)

An operator looking at the Sources screen's Subscriptions list wants each
saved basic-search URL to render as a clear, human-readable summary of
all the filters it carries — keyword, salary, every experience level,
and employment type — instead of a raw opaque URL string, so that at a
glance they can tell two saved searches apart without opening the URL.

**Why this priority**: A saved URL is only useful if the operator can
recognize what it searches for; without readable display, the saved
subscriptions become indistinguishable. But the search itself already
delivers value even if the label is terse.

**Independent Test**: Save the Node.js (exp 2y–5y) and Golang (exp 1y–3y)
basic-search URLs, then open the Subscriptions list and confirm each row
shows its keyword, salary, and experience level information distinctly,
making the two rows visually distinguishable.

**Acceptance Scenarios**:

1. **Given** a saved basic-search subscription with all filters set,
   **When** the operator views its row in the Subscriptions list,
   **Then** every query parameter present in the saved URL is reflected
   in the displayed summary — at minimum: primary keyword, salary,
   experience levels, and employment type.
2. **Given** the Node.js subscription (exp_level 2y, 3y, 4y, 5y) and
   the Golang subscription (exp_level 1y, 2y, 3y), **When** both rows
   are visible, **Then** the experience levels on each row are displayed
   as a **range** ("2–5 years", "1–3 years") rather than as a comma list,
   because the saved values form a consecutive sequence.
3. **Given** a saved basic-search subscription whose experience levels
   are **not** consecutive (e.g. 1y and 3y only, no 2y), **When** the
   row is displayed, **Then** the levels are shown as a discrete list
   ("1, 3 years"), not collapsed into a misleading range.
4. **Given** a saved `subs/{id}/` dashboard URL (the pre-existing
   Djinni subscription shape), **When** its row is displayed, **Then**
   it keeps its existing display behavior and is not broken by the new
   basic-search display path.

---

### User Story 3 - Distinguish the two Djinni search modes when saving and running (Priority: P3)

An operator managing many Djinni subscriptions wants the system to
clearly separate the two supported Djinni search modes — the logged-in
`subs/{id}/` dashboard mode and the public basic-search mode — so that
the right fetch path and the right display summary are used for each,
and a URL saved under one mode is never silently interpreted as the
other.

**Why this priority**: This is about correctness and clarity over time
as many subscriptions accumulate, but the feature already works for a
small number of saved searches where the URL itself disambiguates.

**Independent Test**: Save one `subs/{id}/` URL and one basic-search
URL, trigger runs for both, and confirm each took the correct fetch path
(the dashboard URL paginated through `subs/{id}/`, the basic-search URL
paginated through `djinni.co/jobs/?...`) and each row used the matching
display summary.

**Acceptance Scenarios**:

1. **Given** the operator saves a `djinni.co/jobs/?search_type=basic-search&...`
   URL, **When** the system stores it, **Then** it is recognized as the
   basic-search mode and run with the public `/jobs/` pagination path.
2. **Given** the operator saves a `djinni.co/my/dashboard/subs/{id}/`
   URL, **When** the system stores it, **Then** it is recognized as the
   existing dashboard mode and run with the authenticated subs pagination
   path, unchanged from today.
3. **Given** a URL that is neither a recognizable basic-search URL nor a
   recognizable `subs/{id}/` URL, **When** the operator saves it as a
   Djinni subscription, **Then** saving is rejected with a human-readable
   reason at save time, not deferred to fail at run time.

---

### Edge Cases

- **Single-page basic-search results**: a search that returns fewer
  listings than one full page (the Golang example) completes after page
  1 and is reported as a successful run with the actual count, never as
  an error and never as a loop.
- **Empty basic-search results (zero listings)**: the run completes
  successfully with a count of zero, matching the existing zero-results
  behavior of other Djinni modes, rather than being reported as a
  failure.
- **Repeated `exp_level` values in the URL** (e.g. `exp_level=2y&exp_level=2y`):
  duplicates are collapsed for both run execution and display without
  raising an error.
- **Consecutive `exp_level` values out of order in the URL**
  (e.g. `3y&1y&2y`): the display still recognizes the consecutive set
  and renders it as "1–3 years"; the run issues the levels in the order
  the URL carries them.
- **Missing optional filters** (a basic-search URL with only
  `search_type` and `primary_keyword`, no salary, no exp_level, no
  employment): the run executes against what is present and the display
  omits the absent filters cleanly rather than showing blanks or "null".
- **Basic-search URL with `page=N` already present** when saved: the
  run starts from page 1, ignoring the saved `page` value, so a deep
  link is not mistaken for a starting page.
- **Djinni blocks or challenges the public `/jobs/` request** (rate
  limit, anti-bot): the run ends with a recorded failure and a
  human-readable reason distinct from "no results", following the same
  failure posture already used for the dashboard mode; listings
  collected before the block are retained.
- **Response shape changes upstream** so no card parses: the run reports
  a distinguishable failure (results returned but none interpretable),
  not confused with "no matching jobs" or "blocked".
- **Same job returned by both a basic-search subscription and a
  `subs/{id}/` subscription**: the feed deduplicates to a single entry
  per existing behavior; Djinni is retained as the origin.
- **Saved URL is a basic-search URL with an unrecognized extra query
  parameter**: the unrecognized parameter is preserved in the saved URL
  and re-issued by the run verbatim, but is not interpreted or
  displayed; it does not cause a save rejection.
- **Long-running run interrupted** (cancellation, shutdown, timeout):
  listings gathered before the interruption are retained and the run is
  recorded as partial.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept a public Djinni basic-search URL
  (`djinni.co/jobs/?search_type=basic-search&...`) as a valid Djinni
  subscription, distinct from the existing `subs/{id}/` dashboard
  subscription shape.
- **FR-002**: System MUST recognize which of the two Djinni search modes
  a saved URL belongs to and route the run to the matching fetch path
  (public `/jobs/` pagination for basic-search, authenticated `subs/{id}/`
  pagination for the dashboard mode).
- **FR-003**: When executing a basic-search subscription, the system
  MUST issue the saved URL exactly — preserving `search_type`,
  `primary_keyword`, `salary`, every `exp_level` value, `employment`,
  and any other query parameters present — and MUST start pagination
  from page 1, ignoring any `page` value carried in the saved URL.
- **FR-004**: System MUST correctly complete a basic-search run that
  has only a single page of results: it MUST stop after that page,
  MUST NOT loop, and MUST report the run as successful with the actual
  count rather than as an error.
- **FR-005**: System MUST reuse the existing login-aware fetch path,
  page cap, empty-page stop, and redirect-loop guard used by the
  `subs/{id}/` dashboard mode for basic-search pagination, so both
  modes share the same reliability guarantees.
- **FR-006**: System MUST reuse the existing job-card parsing,
  enrichment, deduplication, scoring, and generation pipelines for
  basic-search listings, with no Djinni-specific exception beyond the
  fetch path.
- **FR-007**: System MUST reject, at save time with a human-readable
  reason, a Djinni subscription URL that is neither a recognizable
  basic-search URL nor a recognizable `subs/{id}/` dashboard URL.
- **FR-008**: On the dashboard, every query parameter present in a saved
  basic-search URL MUST be reflected in that subscription's display
  summary — at minimum primary keyword, salary, experience levels, and
  employment type.
- **FR-009**: When the experience levels in a saved basic-search URL
  form a consecutive sequence (e.g. 1y, 2y, 3y), the display MUST render
  them as a single range ("1–3 years"); when they are not consecutive
  (e.g. 1y, 3y), the display MUST render them as a discrete list
  ("1, 3 years").
- **FR-010**: The display MUST deduplicate repeated experience-level
  values and MUST treat the set of levels as a set when deciding
  range-vs-list, independent of the order the values appear in the URL.
- **FR-011**: The display of a basic-search subscription MUST be
  visually distinguishable from the display of a `subs/{id}/` dashboard
  subscription, so an operator can tell the two Djinni modes apart at a
  glance.
- **FR-012**: Display of a basic-search subscription MUST cleanly omit
  filters that are absent from the saved URL, rather than showing blank
  values or "null" placeholders.
- **FR-013**: The new basic-search mode MUST NOT change the behavior of
  the existing `subs/{id}/` dashboard mode in any way — both saving and
  running dashboard URLs must remain unchanged.
- **FR-014**: System MUST treat a Djinni block/challenge during a
  basic-search run as a distinct, reported failure mode (per the
  existing Djinni failure posture) rather than retrying aggressively or
  attempting to bypass the block.
- **FR-015**: System MUST pace basic-search requests at least as
  conservatively as the existing Djinni mode and MUST bound the number
  of pages a single basic-search run can fetch to the same cap already
  enforced for the dashboard mode.
- **FR-016**: Ingested basic-search listings MUST flow through the same
  downstream matching, scoring, and application-material generation
  paths as listings from existing sources, with no basic-search-specific
  exception.
- **FR-017**: System MUST NOT submit applications, send messages, or
  take any action on a basic-search listing on the user's behalf; the
  basic-search mode, like the dashboard mode, is discovery-only.
- **FR-018**: Saving a basic-search subscription MUST NOT require a
  Djinni login; the basic-search mode executes against publicly visible
  `/jobs/` pages, and a saved basic-search URL MUST run successfully
  even when no Djinni session credentials are configured.

### Key Entities *(include if data involved)*

- **Djinni Subscription (basic-search)**: a saved public
  `djinni.co/jobs/?search_type=basic-search&...` URL and its parsed
  filters (primary keyword, salary, experience-level set, employment
  type), managed through the same Subscriptions flow as the existing
  `subs/{id}/` dashboard shape.
- **Djinni Search Mode**: the discriminator (public basic-search vs.
  logged-in dashboard) that selects the fetch path and the display
  summary for a saved Djinni subscription.
- **Experience-Level Range**: the display representation of a saved
  basic-search URL's `exp_level` set — collapsed to a single inclusive
  range when the set is consecutive, listed discretely otherwise.
- **Normalized Job Listing**: a Djinni basic-search posting converted
  into the product's common job shape; identical entity to the one
  produced by the existing Djinni modes, attributed to Djinni.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can save the Node.js and Golang basic-search
  example URLs, trigger a manual run for each, and see Djinni listings
  in the job feed for each within 5 minutes, with no code change or
  redeploy.
- **SC-002**: A basic-search run whose results fit on a single page (the
  Golang example) completes after that page and is reported successful
  within one run cycle; 100% of single-page basic-search runs end
  cleanly without a pagination loop or a "failed" status.
- **SC-003**: For every saved basic-search subscription, the Subscriptions
  list exposes every filter present in the saved URL; a reviewer can
  identify the keyword, salary, experience levels, and employment type of
  a saved basic-search URL from its row alone in under 5 seconds, without
  opening the URL.
- **SC-004**: 100% of saved basic-search URLs whose experience levels form
  a consecutive sequence are displayed as a single range, and 100% whose
  levels are not consecutive are displayed as a discrete list — verified
  against the Node.js (2y–5y → "2–5 years"), Golang (1y–3y → "1–3
  years"), and non-consecutive (1y, 3y → "1, 3 years") shapes.
- **SC-005**: Re-running the same basic-search subscription immediately
  after a successful run adds zero new feed entries — 100% of
  already-known listings are recognized as duplicates.
- **SC-006**: When Djinni blocks or challenges a basic-search run, the
  subscription's status shows unhealthy with a human-readable reason
  within one run cycle, and no other source's runs are affected.
- **SC-007**: 100% of URLs saved as Djinni subscriptions that are neither
  a recognizable basic-search URL nor a recognizable `subs/{id}/`
  dashboard URL are rejected at save time with a stated reason, and none
  reach a run.
- **SC-008**: Adding the basic-search mode does not change the run or
  display behavior of any existing `subs/{id}/` dashboard subscription —
  verified by running a dashboard subscription before and after the
  feature ships and observing identical outcomes.
- **SC-009**: A basic-search subscription runs successfully with no
  Djinni login configured — its run completes against the public
  `/jobs/` page without requiring session credentials.

## Assumptions

- The new basic-search mode is added alongside the existing Djinni
  dashboard (`subs/{id}/`) mode; no existing Djinni subscription mode is
  removed or changed in behavior by this feature.
- Basic-search listings reuse the existing job-feed, deduplication,
  matching, scoring, and generation pipelines unchanged — this feature
  adds a search mode and a display summary, not a new pipeline.
- A Djinni login is **not** required for the basic-search mode: the
  public `djinni.co/jobs/?search_type=basic-search&...` page is reachable
  anonymously, and a basic-search subscription must run successfully with
  no `DJINNI_EMAIL`/`DJINNI_PASSWORD` configured. The logged-in session
  is reused opportunistically when present (as today) but not required
  for this mode.
- The "search mode" concept here is Djinni-specific: it distinguishes
  the public basic-search URL from the logged-in dashboard URL. It is
  not a new top-level product concept and does not change the
  cross-source `SearchQuery` shape used by other sources.
- Operators save basic-search subscriptions the same way they save
  `subs/{id}/` subscriptions today — pasting a URL into the existing
  New Subscription form on the Sources screen — with the system
  recognizing the URL shape rather than adding a separate UI.
- Experience-level values use Djinni's `Ny` notation (1y, 2y, 3y, 4y,
  5y, ...). "Consecutive" means the set of year numbers, ignoring order
  and duplicates, forms an unbroken integer run; the display renders the
  lowest and highest inclusive bounds.
- Salary is the single `salary` query parameter Djinni uses for the
  minimum monthly net in USD; it is displayed as given (e.g. "3000"),
  without currency conversion or normalization, consistent with the
  best-effort display posture for other sources.
- Only listing discovery is in scope; applying, messaging employers, and
  syncing Djinni application status are out of scope, per the product's
  no-auto-apply rule.
- Upstream response shape and anti-bot measures are expected to change
  over time; the basic-search mode, like the dashboard mode, is treated
  as best-effort, and its failures must not fail the overall ingestion
  cycle.
- Seed/demo data and management-screen coverage for the basic-search
  mode follow the same conventions as existing Djinni modes.