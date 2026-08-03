> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/job-sources.md`](../../domains/job-sources.md) — read that first.

---
# Feature Specification: Djinni Preset-Search Rewrite

**Feature Branch**: `016-djinni-preset-search-rewrite`

**Created**: 2026-07-28

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "lets rewrite implementation of djinni adapter. the only way how I'll use it is by search presets (example: https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote), that way dont require auth and sometimes need pagination. all other ways for djinni is legacy and should be delete"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run a Djinni preset search URL end-to-end with pagination (Priority: P1)

A job seeker saves a public Djinni basic-search preset URL — the kind
produced by clicking filters on `djinni.co/jobs/`, e.g.
`https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote`
— and the system runs it on the same schedule as any other subscription,
fetching every result page when more than one is available and stopping
cleanly after the single page when only one exists. No Djinni login is
required, because the basic-search preset page is publicly reachable.

**Why this priority**: This is the entire point of the rewrite. The
preset-search path must fetch and paginate correctly without auth before
any display or cleanup work matters.

**Independent Test**: Save the example Golang preset URL, trigger a
manual run, and confirm Djinni listings appear in the job feed with the
run reported successful — including the single-page case completing
without looping.

**Acceptance Scenarios**:

1. **Given** the operator pastes a `djinni.co/jobs/?search_type=basic-search&...`
   URL into the New Subscription form, **When** they save it, **Then** it
   is accepted as a valid Djinni subscription.
2. **Given** a saved preset-search subscription, **When** the run
   executes it, **Then** the system issues the URL exactly as saved —
   preserving `search_type`, `primary_keyword`, `salary`, every
   `exp_level` value, `employment`, and any other query parameters —
   and ingests the listings the page returns.
3. **Given** a preset URL whose results fit on a single page (the Golang
   example), **When** the run executes it, **Then** the run completes
   successfully after that single page and does not loop, retry, or
   report a failure.
4. **Given** a preset URL with multiple result pages, **When** the run
   executes it, **Then** the system walks each subsequent page until an
   empty page or the existing page cap fires, reusing the shared fetch
   path used by other sources.
5. **Given** a preset-search subscription with no Djinni login
   configured, **When** the run executes, **Then** it completes against
   the public `/jobs/` page without requiring session credentials.

---

### User Story 2 - See every preset filter on the subscription in the dashboard (Priority: P2)

An operator looking at the Sources screen's Subscriptions list wants
each saved preset URL to render as a clear, human-readable summary of
all the filters it carries — keyword, salary, every experience level,
and employment type — instead of a raw opaque URL string, so that at a
glance they can tell two saved presets apart without opening the URL.

**Why this priority**: A saved URL is only useful if the operator can
recognize what it searches for; without readable display the saved
presets become indistinguishable. But the search itself already delivers
value even if the label is terse.

**Independent Test**: Save two preset URLs that differ only in
`exp_level` values, open the Subscriptions list, and confirm each row
shows its keyword, salary, and experience level information distinctly,
making the two rows visually distinguishable.

**Acceptance Scenarios**:

1. **Given** a saved preset-search subscription with all filters set,
   **When** the operator views its row in the Subscriptions list,
   **Then** every query parameter present in the saved URL is reflected
   in the displayed summary — at minimum: primary keyword, salary,
   experience levels, and employment type.
2. **Given** a preset whose experience levels form a consecutive
   sequence (e.g. 1y, 2y, 3y), **When** the row is displayed, **Then**
   the levels are shown as a **range** ("1–3 years") rather than a
   comma list.
3. **Given** a preset whose experience levels are **not** consecutive
   (e.g. 1y and 3y only), **When** the row is displayed, **Then** the
   levels are shown as a discrete list ("1, 3 years"), not collapsed
   into a misleading range.
4. **Given** a preset URL with an optional filter absent (no salary, no
   exp_level, or no employment), **When** the row is displayed, **Then**
   the absent filter is omitted cleanly rather than shown as blank or
   "null".

---

### User Story 3 - All legacy Djinni paths are removed (Priority: P3)

A maintainer wants the codebase to contain only the preset-search path
for Djinni — no logged-in dashboard (`subs/{id}/`) mode, no session
login, no dual-mode routing, no legacy display branch — so that the
adapter is smaller, has one failure posture, and a saved Djinni
subscription is unambiguously a preset search.

**Why this priority**: Correctness and clarity over time once the rewrite
shipped, but the feature already works for new preset subscriptions even
before the legacy code is gone; deletion is cleanup that must not break
the preset path.

**Independent Test**: After the rewrite, grep the Djinni adapter for
`subs/{id}/`, session login, and dual-mode dispatch and confirm none
remain; then trigger a preset run and confirm it still succeeds.

**Acceptance Scenarios**:

1. **Given** the rewrite has shipped, **When** a maintainer inspects the
   Djinni adapter, **Then** there is a single search path (public
   preset-search pagination) and no dashboard `subs/{id}/` fetch path
   and no session-login path.
2. **Given** a `djinni.co/my/dashboard/subs/{id}/` URL, **When** the
   operator tries to save it as a Djinni subscription, **Then** saving
   is rejected at save time with a human-readable reason stating that
   only preset-search URLs are supported.
3. **Given** an existing `subs/{id}/` Djinni subscription saved before
   the rewrite, **When** the rewrite ships, **Then** that subscription
   is no longer runnable and is surfaced to the operator as a stale
   subscription requiring re-save as a preset URL, rather than silently
   failing at run time.
4. **Given** the rewrite has shipped, **When** a preset-search
   subscription runs, **Then** it succeeds using only the remaining
   preset path — confirming the deletions did not regress the preset
   flow.

---

### Edge Cases

- **Single-page preset results**: a search that returns fewer listings
  than one full page (the Golang example) completes after page 1 and is
  reported as a successful run with the actual count, never as an error
  and never as a loop.
- **Empty preset results (zero listings)**: the run completes
  successfully with a count of zero, matching the zero-results behavior
  of other sources, rather than being reported as a failure.
- **Repeated `exp_level` values** (e.g. `exp_level=2y&exp_level=2y`):
  duplicates are collapsed for both run execution and display without
  raising an error.
- **Consecutive `exp_level` values out of order** (e.g. `3y&1y&2y`):
  the display still recognizes the consecutive set and renders it as
  "1–3 years"; the run issues the levels in the order the URL carries
  them.
- **Missing optional filters** (a preset with only `search_type` and
  `primary_keyword`): the run executes against what is present and the
  display omits the absent filters cleanly rather than showing blanks
  or "null".
- **Preset URL with `page=N` already present** when saved: the run
  starts from page 1, ignoring the saved `page` value.
- **Djinni blocks or challenges the public `/jobs/` request** (rate
  limit, anti-bot): the run ends with a recorded failure and a
  human-readable reason distinct from "no results"; listings collected
  before the block are retained.
- **Response shape changes upstream** so no card parses: the run
  reports a distinguishable failure (results returned but none
  interpretable), not confused with "no matching jobs" or "blocked".
- **Saved URL is a preset URL with an unrecognized extra query
  parameter**: the unrecognized parameter is preserved in the saved
  URL and re-issued by the run verbatim, but is not interpreted or
  displayed; it does not cause a save rejection.
- **Configuration references to the removed login** (`DJINNI_EMAIL`/
  `DJINNI_PASSWORD`, session cookies): are removed cleanly with no
  dangling references, and preset runs do not consult them.
- **Long-running run interrupted** (cancellation, shutdown, timeout):
  listings gathered before the interruption are retained and the run is
  recorded as partial.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST accept a public Djinni preset-search URL
  (`djinni.co/jobs/?search_type=basic-search&...`) as a valid Djinni
  subscription — the only supported Djinni subscription shape after the
  rewrite.
- **FR-002**: When executing a preset subscription, the system MUST
  issue the saved URL exactly — preserving `search_type`,
  `primary_keyword`, `salary`, every `exp_level` value, `employment`,
  and any other query parameters present — and MUST start pagination
  from page 1, ignoring any `page` value carried in the saved URL.
- **FR-003**: System MUST correctly complete a preset run that has only
  a single page of results: it MUST stop after that page, MUST NOT loop,
  and MUST report the run as successful with the actual count rather
  than as an error.
- **FR-004**: System MUST correctly paginate a preset run that has
  multiple result pages — walking each subsequent page until an empty
  page or the existing page cap fires — reusing the shared fetch,
  page-cap, empty-page-stop, and redirect-loop-guard behavior used by
  other sources.
- **FR-005**: System MUST run a preset subscription successfully with
  no Djinni login configured; the preset mode executes against publicly
  visible `/jobs/` pages and MUST NOT require session credentials.
- **FR-006**: System MUST remove the legacy logged-in dashboard
  (`subs/{id}/`) fetch path, the Djinni session-login path, and any
  dual-mode routing/dispatch for Djinni — only the preset-search path
  may remain in the codebase.
- **FR-007**: System MUST remove configuration fields, environment
  variables, and seed/demo data specific to the removed Djinni login
  (e.g. `DJINNI_EMAIL`/`DJINNI_PASSWORD`, session cookies) without
  dangling references, and preset runs MUST NOT consult them.
- **FR-008**: System MUST reject, at save time with a human-readable
  reason, a Djinni subscription URL that is not a recognizable
  preset-search URL — including former `subs/{id}/` dashboard URLs,
  which MUST now be rejected with a reason stating that only
  preset-search URLs are supported.
- **FR-009**: System MUST run a one-time migration that deletes every
  pre-existing `subs/{id}/` Djinni subscription (saved before the
  rewrite); the deletion is recorded so the operator can see what was
  removed, and no `subs/{id}/` subscription remains to silently fail at
  run time.
- **FR-010**: On the dashboard, every query parameter present in a
  saved preset URL MUST be reflected in that subscription's display
  summary — at minimum primary keyword, salary, experience levels, and
  employment type.
- **FR-011**: When the experience levels in a saved preset URL form a
  consecutive sequence (e.g. 1y, 2y, 3y), the display MUST render them
  as a single range ("1–3 years"); when they are not consecutive
  (e.g. 1y, 3y), the display MUST render them as a discrete list
  ("1, 3 years").
- **FR-012**: The display MUST deduplicate repeated experience-level
  values and MUST treat the set of levels as a set when deciding
  range-vs-list, independent of the order the values appear in the URL.
- **FR-013**: Display of a preset subscription MUST cleanly omit
  filters that are absent from the saved URL, rather than showing blank
  values or "null" placeholders.
- **FR-014**: System MUST reuse the existing job-card parsing,
  enrichment, deduplication, scoring, and generation pipelines for
  preset listings, with no Djinni-specific exception beyond the fetch
  path.
- **FR-015**: System MUST treat a Djinni block/challenge during a
  preset run as a distinct, reported failure mode (per the existing
  Djinni failure posture) rather than retrying aggressively or
  attempting to bypass the block.
- **FR-016**: System MUST pace preset requests at least as
  conservatively as the existing Djinni mode and MUST bound the number
  of pages a single preset run can fetch to the same cap already
  enforced for the source.
- **FR-017**: Ingested preset listings MUST flow through the same
  downstream matching, scoring, and application-material generation
  paths as listings from existing sources, with no preset-specific
  exception.
- **FR-018**: System MUST NOT submit applications, send messages, or
  take any action on a preset listing on the user's behalf; the preset
  mode, like every source, is discovery-only.
- **FR-019**: The rewrite MUST NOT change the run or display behavior
  of any non-Djinni source.

### Key Entities *(include if feature involves data)*

- **Djinni Subscription (preset-search)**: a saved public
  `djinni.co/jobs/?search_type=basic-search&...` URL and its parsed
  filters (primary keyword, salary, experience-level set, employment
  type), managed through the same Subscriptions flow as other sources.
  After the rewrite, the only supported Djinni subscription shape.
- **Experience-Level Range**: the display representation of a saved
  preset URL's `exp_level` set — collapsed to a single inclusive range
  when the set is consecutive, listed discretely otherwise.
- **Normalized Job Listing**: a Djinni preset-search posting converted
  into the product's common job shape; identical entity to the one
  produced by existing sources, attributed to Djinni.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can save the Golang preset example URL,
  trigger a manual run, and see Djinni listings in the job feed within
  5 minutes, with no code change or redeploy — and with no Djinni login
  configured.
- **SC-002**: A preset run whose results fit on a single page (the
  Golang example) completes after that page and is reported successful
  within one run cycle; 100% of single-page preset runs end cleanly
  without a pagination loop or a "failed" status.
- **SC-003**: For every saved preset subscription, the Subscriptions
  list exposes every filter present in the saved URL; a reviewer can
  identify the keyword, salary, experience levels, and employment type
  of a saved preset URL from its row alone in under 5 seconds, without
  opening the URL.
- **SC-004**: 100% of saved preset URLs whose experience levels form a
  consecutive sequence are displayed as a single range, and 100% whose
  levels are not consecutive are displayed as a discrete list —
  verified against consecutive (1y–3y → "1–3 years") and
  non-consecutive (1y, 3y → "1, 3 years") shapes.
- **SC-005**: Re-running the same preset subscription immediately after
  a successful run adds zero new feed entries — 100% of already-known
  listings are recognized as duplicates.
- **SC-006**: When Djinni blocks or challenges a preset run, the
  subscription's status shows unhealthy with a human-readable reason
  within one run cycle, and no other source's runs are affected.
- **SC-007**: 100% of URLs saved as Djinni subscriptions that are not a
  recognizable preset-search URL — including former `subs/{id}/`
  dashboard URLs — are rejected at save time with a stated reason, and
  none reach a run.
- **SC-008**: After the rewrite, the Djinni adapter contains a single
  search path; a maintainer search of the adapter and its configuration
  surfaces zero references to `subs/{id}/`, session login, or dual-mode
  dispatch — verified by static inspection of the shipped adapter and
  config.
- **SC-009**: A one-time migration deletes 100% of pre-existing
  `subs/{id}/` Djinni subscriptions on ship, and a recorded list of
  the deleted subscriptions is available so the operator can recreate
  them as preset URLs; zero `subs/{id}/` subscriptions remain runnable
  or silently failing after the migration runs.
- **SC-010**: Running any non-Djinni source before and after the
  rewrite produces identical outcomes — the rewrite does not change
  other sources' behavior.

## Assumptions

- The rewrite **replaces** the existing Djinni implementation: the
  logged-in dashboard (`subs/{id}/`) mode, the session-login path, and
  any dual-mode routing are deleted, not merely deprecated. Only the
  public preset-search path remains.
- A Djinni login is **not** required for the preset mode: the public
  `djinni.co/jobs/?search_type=basic-search&...` page is reachable
  anonymously, and a preset subscription must run successfully with no
  `DJINNI_EMAIL`/`DJINNI_PASSWORD` configured. The existing session
  machinery is removed rather than kept "opportunistic".
- Preset listings reuse the existing job-feed, deduplication, matching,
  scoring, and generation pipelines unchanged — this feature rewrites
  the Djinni adapter and its display, not a new pipeline.
- Operators save preset subscriptions the same way they save any
  subscription today — pasting a URL into the existing New Subscription
  form on the Sources screen — with the system recognizing the
  preset-search URL shape.
- Experience-level values use Djinni's `Ny` notation (1y, 2y, 3y, 4y,
  5y, ...). "Consecutive" means the set of year numbers, ignoring order
  and duplicates, forms an unbroken integer run; the display renders the
  lowest and highest inclusive bounds.
- Salary is the single `salary` query parameter Djinni uses for the
  minimum monthly net in USD; it is displayed as given (e.g. "1500"),
  without currency conversion or normalization, consistent with the
  best-effort display posture for other sources.
- Only listing discovery is in scope; applying, messaging employers,
  and syncing Djinni application status are out of scope, per the
  product's no-auto-apply rule.
- Existing display/parsing logic carried over verbatim from the prior
  basic-search work (e.g. `djinniSearchSummary` range-vs-list rules) is
  reused rather than re-derived; behavior-equivalent is acceptable.
- Upstream response shape and anti-bot measures are expected to change
  over time; the preset mode, like any source, is treated as
  best-effort, and its failures must not fail the overall ingestion
  cycle.
- Pre-existing `subs/{id}/` subscriptions are **deleted** by a one-time
  migration on ship; the dashboard URL cannot be losslessly converted to
  a preset URL, so the operator recreates them manually from a recorded
  list of what was removed.