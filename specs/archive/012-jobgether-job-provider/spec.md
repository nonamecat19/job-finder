> **ARCHIVED — SHIPPED (see drift note).**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/job-sources.md`](../../domains/job-sources.md) — read that first.
>
> **The Jobgether adapter is wired for enrichment only and is absent from the ingest registry — FR-001/002/005/006 are not currently met. See the drift note in the domain doc.**

---
# Feature Specification: Jobgether Job Source

**Feature Branch**: `012-jobgether-job-provider`

**Created**: 2026-07-25

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "add Jobgether job provider"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover Jobgether listings alongside existing sources (Priority: P1)

A job seeker who already receives listings from Djinni, DOU, Indeed, RemoteOK, Glassdoor,
JobLeads, Wellfound, and Himalayas wants Jobgether's remote listings to appear in the same job
feed, deduplicated and scored the same way, without having to browse Jobgether manually.

**Why this priority**: This is the core value — Jobgether aggregates remote listings across
many employers, including its own match-percentage signal. Without listings flowing into the
feed, nothing else in this feature matters.

**Independent Test**: Enable the Jobgether source, trigger a run, and confirm new job records
appear in the feed tagged with Jobgether as their origin, each with title, company, location,
remote flag, posting URL, and description.

**Acceptance Scenarios**:

1. **Given** the Jobgether source is enabled and a search configuration exists, **When** a
   source run is triggered, **Then** Jobgether listings matching that configuration are added
   to the job feed and attributed to Jobgether as their origin.
2. **Given** a Jobgether listing was already ingested in a previous run, **When** the same run
   is repeated, **Then** the listing is recognized as already known and is not duplicated in
   the feed.
3. **Given** Jobgether listings are in the feed, **When** the user filters or sorts the feed by
   source, **Then** Jobgether appears as a selectable source alongside the existing sources.

---

### User Story 2 - Manage the Jobgether source like any other source (Priority: P2)

An operator opens the Sources screen and manages Jobgether exactly as they manage the other
sources: enable/disable it, edit its configuration, run it on demand, test its health, and see
its last run status and result counts.

**Why this priority**: Parity in the management surface is what makes the source usable and
debuggable day to day, but the feature still delivers value if the source is only runnable on
its normal schedule.

**Independent Test**: From the Sources screen, save a Jobgether subscription, toggle Jobgether
off and on, run a health test, trigger a manual run, and verify run history and counts
update — all without touching other sources' behavior.

**Acceptance Scenarios**:

1. **Given** the Sources screen is open, **When** the operator views the source list, **Then**
   Jobgether is listed with its enabled state, health state, and last run summary.
2. **Given** the operator saves a Jobgether subscription (a role/location/category search
   configuration), **When** they save it, **Then** it is accepted and becomes the
   configuration the next Jobgether run executes against; an input that is not a recognizable
   Jobgether subscription is rejected with a stated reason.
3. **Given** Jobgether is enabled, **When** the operator triggers a health test, **Then** the
   system reports whether Jobgether is currently reachable and usable, with a human-readable
   reason on failure.
4. **Given** Jobgether is disabled, **When** a scheduled ingestion cycle runs, **Then** no
   Jobgether requests are made and no Jobgether listings are added.

---

### User Story 3 - Enrich Jobgether listings with full posting detail (Priority: P3)

A user opens a Jobgether listing from the feed and sees the full job description and
qualifications — not just the summary fields returned by the initial listing fetch — so that
matching, scoring, and generated application materials are grounded in the complete posting.

**Why this priority**: Grounded generation depends on the full description, but the feed is
already useful with summary-level data, so this can follow the initial ingestion slice.

**Independent Test**: Ingest one Jobgether listing, run enrichment for it, and confirm the
stored description and qualifications are captured in full, with posting date resolved.

**Acceptance Scenarios**:

1. **Given** a Jobgether listing was ingested with summary data, **When** enrichment runs for
   that listing, **Then** the stored posting contains the full description text and a
   resolved posting date.
2. **Given** a Jobgether listing whose posting is no longer available, **When** enrichment runs
   for it, **Then** the listing is marked as unavailable and the summary data already captured
   is preserved rather than discarded.

---

### Edge Cases

- **Source blocks or throttles the request** (rate limiting, access denial): the run ends with
  a recorded failure and a human-readable reason; already-ingested listings from earlier in
  the run are kept, and the source's health state reflects the failure.
- **Zero results** for a valid configuration: the run completes successfully with a count of
  zero rather than being reported as an error.
- **Response shape changes upstream** so nothing parses: the run reports a failure with a
  distinguishable reason (results were returned but none could be interpreted), so the cause
  is not confused with "no matching jobs".
- **Same job posted on Jobgether and on another source** (Jobgether frequently mirrors listings
  originally posted on an employer's own careers page or another board): the feed shows a
  single entry per distinct posting per existing deduplication behavior; the Jobgether origin
  is retained on whichever record is kept.
- **Jobgether's own AI match-percentage score is present on a listing**: it is captured as
  descriptive metadata only and MUST NOT replace or influence this product's own scoring.
- **Listing has no salary range published**: the listing is still ingested with that field
  empty rather than being dropped.
- **Long-running run interrupted** (cancellation, shutdown, timeout): listings gathered before
  the interruption are retained and the run is recorded as partial.
- **Saved subscription is not a recognizable Jobgether configuration** (wrong site, malformed,
  or unknown role/category combination): saving it is rejected with a human-readable reason at
  configuration time, not silently at run time.
- **Upstream response exceeds the expected listing count for a single fetch**: the run keeps
  all listings returned in that fetch, records how many it collected, and reports success.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST make Jobgether available as a first-class job source, selectable and
  attributable in the same places other sources are (feed source attribution, source filters,
  saved-search source selection).
- **FR-002**: System MUST retrieve Jobgether listings for a user-defined search configuration
  and add them to the job feed.
- **FR-003**: Each ingested Jobgether listing MUST carry, when the source publishes it: title,
  company, location text, remote indicator, raw salary text, posting URL, description, and a
  stable per-listing identifier.
- **FR-004**: System MUST recognize previously ingested Jobgether listings across runs and not
  create duplicate feed entries for them.
- **FR-005**: System MUST allow the Jobgether source to be enabled and disabled, and MUST make
  no Jobgether requests while it is disabled.
- **FR-006**: System MUST support an on-demand run and an on-demand health check for the
  Jobgether source, reporting outcome and a human-readable reason on failure.
- **FR-007**: System MUST record each Jobgether run's outcome (succeeded, failed, or partial),
  the number of listings found, and the number newly added.
- **FR-008**: System MUST retain listings already collected when a run is interrupted or fails
  partway, and MUST record the run as partial rather than discarding results.
- **FR-009**: System MUST support retrieving the full detail of an individual Jobgether listing
  so that description and posting date can be completed after initial ingestion, when the
  initial fetch did not already include them.
- **FR-010**: System MUST pace its Jobgether requests so that a single run does not issue
  requests faster than one every 500 milliseconds, and MUST bound the number of requests a
  single run can make.
- **FR-011**: System MUST distinguish, in run outcomes, between "source returned no matching
  listings" and "source returned content that could not be interpreted".
- **FR-012**: Ingested Jobgether listings MUST flow through the same downstream matching,
  scoring, and application-material generation paths as listings from existing sources, with
  no Jobgether-specific exception, and MUST NOT substitute Jobgether's own match score for this
  product's scoring.
- **FR-013**: System MUST NOT submit applications, send messages, or take any action on a
  Jobgether listing on the user's behalf; Jobgether integration is discovery-only.
- **FR-014**: System MUST accept a Jobgether search configuration as an operator-saved
  subscription, managed through the same subscription flow already used for existing sources.
- **FR-015**: System MUST reject a saved subscription that is not a recognizable Jobgether
  configuration, with a human-readable reason, rather than saving it and failing at run time.
- **FR-016**: The Jobgether source MUST expose its own identity in run records, source
  attribution, and health status, distinct from any other source.
- **FR-017**: System MUST identify its requests to Jobgether with a descriptive client
  identifier, consistent with the conservative-access posture used for other sources.

### Key Entities *(include if data involved)*

- **Job Source (Jobgether)**: the registered, per-source configuration record — enabled state,
  configuration values, health state, and last run summary.
- **Source Subscription**: a saved Jobgether search configuration (role, location, category)
  that a run executes against; one source may have several.
- **Normalized Job Listing**: a Jobgether posting converted into the product's common job shape
  (title, company, location, remote flag, salary text, URL, description, posting date, source
  origin, stable identifier).
- **Source Run**: a record of one Jobgether execution — start/end, outcome, listings found,
  listings newly added, failure reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can save a Jobgether subscription, enable the source, and see
  Jobgether listings in the job feed within 5 minutes, with no code change or redeploy.
- **SC-002**: A single Jobgether run against a typical subscription returns at least 20
  distinct listings, or all available listings if fewer than 20 exist.
- **SC-003**: Re-running the same Jobgether configuration immediately adds zero new feed
  entries — 100% of already-known listings are recognized as duplicates.
- **SC-004**: At least 95% of ingested Jobgether listings have a non-empty title, company, and
  openable posting URL.
- **SC-005**: When Jobgether is unreachable or blocks the run, the source's status shows
  unhealthy with a human-readable reason within one run cycle, and no other source's runs are
  affected.
- **SC-006**: After enrichment, at least 90% of Jobgether listings have a description longer
  than the summary captured at ingestion time.
- **SC-007**: Adding Jobgether does not increase the median end-to-end ingestion cycle time for
  the existing sources by more than 10%.
- **SC-008**: 100% of saved subscriptions that are not valid Jobgether configurations are
  rejected at save time with a stated reason, and none reach a run.

## Assumptions

- Jobgether is added as an additional source; no existing source is removed, disabled, or
  changed in behavior by this feature.
- Jobgether listings reuse the existing job-feed, deduplication, matching, scoring, and
  generation pipelines unchanged — this feature adds a source, not a new pipeline.
- Jobgether listing and detail pages are publicly viewable without an account for the fields
  this feature needs.
- Jobgether's terms and rate limits are respected via conservative request pacing, a
  descriptive client identifier, and a bounded number of requests per run; aggressive crawling
  and anti-bot evasion are out of scope.
- Operators configure Jobgether the same way they configure existing sources — saving a
  subscription through the existing Sources screen flow.
- Only listing discovery and detail retrieval are in scope; applying, messaging employers, and
  syncing Jobgether application status are out of scope, per the product's no-auto-apply rule.
- Seed/demo data and management-screen coverage for Jobgether follow the same conventions as
  existing sources.
- Upstream response shape is expected to change over time; the source is treated as
  best-effort, and its failures must not fail the overall ingestion cycle.
- Jobgether's proprietary match-percentage score, where present, is stored only as descriptive
  metadata and never feeds this product's own matching/scoring logic.
