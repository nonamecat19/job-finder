> **ARCHIVED — SHIPPED (see drift note).**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/job-sources.md`](../../domains/job-sources.md) — read that first.
>
> **The Indeed adapter is wired for enrichment only and is absent from the ingest registry — FR-001/002/005/006 are not currently met. See the drift note in the domain doc.**

---
# Feature Specification: Indeed Job Source

**Feature Branch**: `002-indeed-job-source`

**Created**: 2026-07-24

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "lets add indeed as a jobs provider similar to djinni and dou"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover Indeed listings alongside existing sources (Priority: P1)

A job seeker who already receives listings from Djinni and DOU wants Indeed listings to
appear in the same job feed, deduplicated and scored the same way, without having to open
Indeed manually.

**Why this priority**: This is the core value — Indeed carries the largest volume of
postings of any source in the product's target markets. Without listings flowing into the
feed, nothing else in this feature matters.

**Independent Test**: Enable the Indeed source, trigger a run, and confirm new job records
appear in the feed tagged with Indeed as their origin, each with title, company, location,
remote flag, posting URL, and description.

**Acceptance Scenarios**:

1. **Given** the Indeed source is enabled and a search configuration exists, **When** a
   source run is triggered, **Then** Indeed listings matching that configuration are added
   to the job feed and attributed to Indeed as their origin.
2. **Given** an Indeed listing was already ingested in a previous run, **When** the same
   run is repeated, **Then** the listing is recognized as already known and is not
   duplicated in the feed.
3. **Given** Indeed listings are in the feed, **When** the user filters or sorts the feed
   by source, **Then** Indeed appears as a selectable source alongside Djinni and DOU.

---

### User Story 2 - Manage the Indeed source like any other source (Priority: P2)

An operator opens the Sources screen and manages Indeed exactly as they manage Djinni and
DOU: enable/disable it, edit its configuration, run it on demand, test its health, and see
its last run status and result counts.

**Why this priority**: Parity in the management surface is what makes the source usable
and debuggable day to day, but the feature still delivers value if the source is only
runnable on its normal schedule.

**Independent Test**: From the Sources screen, save an Indeed search URL, toggle Indeed off
and on, run a health test, trigger a manual run, and verify run history and counts update
— all without touching other sources' behavior.

**Acceptance Scenarios**:

1. **Given** the Sources screen is open, **When** the operator views the source list,
   **Then** Indeed is listed with its enabled state, health state, and last run summary.
2. **Given** the operator pastes an Indeed search URL as a subscription, **When** they save
   it, **Then** it is accepted and becomes the configuration the next Indeed run executes
   against; a URL that is not a valid Indeed search is rejected with a stated reason.
3. **Given** Indeed is enabled, **When** the operator triggers a health test, **Then** the
   system reports whether Indeed is currently reachable and usable, with a human-readable
   reason on failure.
4. **Given** Indeed is disabled, **When** a scheduled ingestion cycle runs, **Then** no
   Indeed requests are made and no Indeed listings are added.

---

### User Story 3 - Enrich Indeed listings with full posting detail (Priority: P3)

A user opens an Indeed listing from the feed and sees the full job description, remote
status, and posting date — not just the short summary shown on the results page — so that
matching, scoring, and generated application materials are grounded in the complete
posting.

**Why this priority**: Grounded generation depends on the full description, but the feed
is already useful with summary-level data, so this can follow the initial ingestion slice.

**Independent Test**: Ingest one Indeed listing, run enrichment for it, and confirm the
stored description grows from summary to full posting text, with remote status and posting
date resolved.

**Acceptance Scenarios**:

1. **Given** an Indeed listing was ingested with only summary data, **When** enrichment
   runs for that listing, **Then** the stored posting contains the full description text
   and a resolved posting date where the source publishes one.
2. **Given** an Indeed listing whose detail page is no longer available, **When**
   enrichment runs for it, **Then** the listing is marked as unavailable and the summary
   data already captured is preserved rather than discarded.

---

### Edge Cases

- **Source blocks the request** (rate limiting, bot challenge, or access denial): the run
  ends with a recorded failure and a human-readable reason; already-ingested listings from
  earlier in the run are kept, and the source's health state reflects the failure.
- **Zero results** for a valid configuration: the run completes successfully with a count
  of zero rather than being reported as an error.
- **Markup or response shape changes upstream** so nothing parses: the run reports a
  failure with a distinguishable reason (results were returned but none could be
  interpreted), so the cause is not confused with "no matching jobs".
- **Same job posted on Indeed and on another source**: the feed shows a single entry per
  distinct posting per existing deduplication behavior; the Indeed origin is retained on
  whichever record is kept.
- **Listing has no salary, no company, or no location**: the listing is still ingested with
  those fields empty rather than being dropped.
- **Aggregated listings that redirect off-site**: the stored posting URL resolves to a page
  a human can open and apply from.
- **Long-running or paginated run interrupted** (cancellation, shutdown, timeout): listings
  gathered before the interruption are retained and the run is recorded as partial.
- **Country/locale-specific listings**: listings from the country implied by the pasted
  search URL are returned, with location text preserved as the source presents it.
- **Pasted URL is not an Indeed search URL** (wrong site, an individual job page, or
  malformed): saving it is rejected with a human-readable reason at configuration time,
  not silently at run time.
- **Pasted search URL yields more results than the pagination bound allows**: the run
  stops at the bound, records how many listings it collected, and reports success rather
  than failure.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST make Indeed available as a first-class job source, selectable and
  attributable in the same places Djinni and DOU are (feed source attribution, source
  filters, saved-search source selection).
- **FR-002**: System MUST retrieve Indeed listings for a user-defined search configuration
  and add them to the job feed.
- **FR-003**: Each ingested Indeed listing MUST carry, when the source publishes it: title,
  company, location, remote indicator, raw salary text, posting URL, description, posting
  date, and a stable per-listing identifier.
- **FR-004**: System MUST recognize previously ingested Indeed listings across runs and not
  create duplicate feed entries for them.
- **FR-005**: System MUST allow the Indeed source to be enabled and disabled, and MUST make
  no Indeed requests while it is disabled.
- **FR-006**: System MUST support an on-demand run and an on-demand health check for the
  Indeed source, reporting outcome and a human-readable reason on failure.
- **FR-007**: System MUST record each Indeed run's outcome (succeeded, failed, or partial),
  the number of listings found, and the number newly added.
- **FR-008**: System MUST retain listings already collected when a run is interrupted or
  fails partway, and MUST record the run as partial rather than discarding results.
- **FR-009**: System MUST support retrieving the full detail of an individual Indeed
  listing so that description, remote status, and posting date can be completed after
  initial ingestion.
- **FR-010**: System MUST pace its Indeed requests so that a single run does not issue
  requests faster than one every 500 milliseconds, and MUST stop paginating a run after a
  bounded number of pages.
- **FR-011**: System MUST distinguish, in run outcomes, between "source returned no
  matching listings" and "source returned content that could not be interpreted".
- **FR-012**: Ingested Indeed listings MUST flow through the same downstream matching,
  scoring, and application-material generation paths as listings from existing sources,
  with no Indeed-specific exception.
- **FR-013**: System MUST NOT submit applications, send messages, or take any action on an
  Indeed listing on the user's behalf; Indeed integration is discovery-only.
- **FR-014**: System MUST honor the country/region implied by the pasted Indeed search URL
  (Indeed serves different listings per country domain) rather than forcing a single fixed
  market.
- **FR-015**: System MUST accept an Indeed search configuration as an operator-pasted
  Indeed search URL, managed through the same subscription flow already used for DOU and
  Djinni; keyword-parameter search is out of scope for this feature.
- **FR-016**: System MUST reject a pasted subscription URL that is not a recognizable
  Indeed search URL, with a human-readable reason, rather than saving it and failing at
  run time.
- **FR-017**: Indeed retrieval MUST be a dedicated, direct Indeed integration owned by this
  system, independent of the existing multi-site discovery sidecar; the sidecar's own
  behavior MUST remain unchanged by this feature.
- **FR-018**: The Indeed source MUST expose its own identity in run records, source
  attribution, and health status, distinct from any other source that also happens to
  surface Indeed postings.

### Key Entities *(include if data involved)*

- **Job Source (Indeed)**: the registered, per-source configuration record — enabled state,
  configuration values (country, search parameters), health state, and last run summary.
- **Source Subscription**: a saved Indeed search configuration that a run executes against;
  one source may have several.
- **Normalized Job Listing**: an Indeed posting converted into the product's common job
  shape (title, company, location, remote flag, salary text, URL, description, posting
  date, source origin, stable identifier).
- **Source Run**: a record of one Indeed execution — start/end, outcome, listings found,
  listings newly added, failure reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can paste an Indeed search URL, enable the source, and see Indeed
  listings in the job feed within 5 minutes, with no code change or redeploy.
- **SC-002**: A single Indeed run against a typical search configuration returns at least
  50 distinct listings, or all available listings if fewer than 50 exist.
- **SC-003**: Re-running the same Indeed configuration immediately adds zero new feed
  entries — 100% of already-known listings are recognized as duplicates.
- **SC-004**: At least 95% of ingested Indeed listings have a non-empty title, company, and
  openable posting URL.
- **SC-005**: When Indeed is unreachable or blocks the run, the source's status shows
  unhealthy with a human-readable reason within one run cycle, and no other source's runs
  are affected.
- **SC-006**: After enrichment, at least 90% of Indeed listings have a description longer
  than the summary captured at ingestion time.
- **SC-007**: Adding Indeed does not increase the median end-to-end ingestion cycle time for
  the existing sources by more than 10%.
- **SC-008**: 100% of pasted subscription URLs that are not valid Indeed search URLs are
  rejected at save time with a stated reason, and none reach a run.

## Assumptions

- Indeed is added as an additional source; no existing source (Djinni, DOU, or any other)
  is removed, disabled, or changed in behavior by this feature.
- Indeed listings reuse the existing job-feed, deduplication, matching, scoring, and
  generation pipelines unchanged — this feature adds a source, not a new pipeline.
- No Indeed user account, login, or authenticated session is required or stored; only
  publicly visible listings are in scope.
- Indeed's terms and rate limits are respected via conservative request pacing and bounded
  pagination; aggressive crawling and anti-bot evasion are out of scope.
- Target country is determined by the pasted search URL, so no separate country setting is
  introduced.
- Operators are able to build the search they want on Indeed itself and paste the resulting
  URL — the same workflow already used for DOU and Djinni subscriptions.
- The existing multi-site discovery sidecar keeps its current Indeed capability; this
  feature neither depends on it nor removes it, and overlap between the two is handled by
  existing deduplication.
- Only listing discovery and detail retrieval are in scope; applying, messaging employers,
  and syncing Indeed application status are out of scope, per the product's no-auto-apply
  rule.
- Seed/demo data and management-screen coverage for Indeed follow the same conventions as
  existing sources.
- Upstream markup and response shapes are expected to change over time; the source is
  treated as best-effort, and its failures must not fail the overall ingestion cycle.
