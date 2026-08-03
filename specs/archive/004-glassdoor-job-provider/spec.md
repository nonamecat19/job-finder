> **ARCHIVED — SHIPPED (see drift note).**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/job-sources.md`](../../domains/job-sources.md) — read that first.
>
> **The Glassdoor adapter is wired for enrichment only and is absent from the ingest registry — FR-001/002/005/006 are not currently met. See the drift note in the domain doc.**

---
# Feature Specification: Glassdoor Job Source

**Feature Branch**: `004-glassdoor-job-provider`

**Created**: 2026-07-24

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "lets add Glassdoor vacancies provider"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover Glassdoor listings alongside existing sources (Priority: P1)

A job seeker who already receives listings from Djinni, DOU, Indeed, and RemoteOK wants
Glassdoor listings to appear in the same job feed, deduplicated and scored the same way,
without having to open Glassdoor manually.

**Why this priority**: This is the core value — Glassdoor is a major listings board with
company-reported data (salary ranges, ratings) other sources may not carry. Without listings
flowing into the feed, nothing else in this feature matters.

**Independent Test**: Enable the Glassdoor source, trigger a run, and confirm new job records
appear in the feed tagged with Glassdoor as their origin, each with title, company, location,
remote flag, posting URL, and description.

**Acceptance Scenarios**:

1. **Given** the Glassdoor source is enabled and a search configuration exists, **When** a
   source run is triggered, **Then** Glassdoor listings matching that configuration are added
   to the job feed and attributed to Glassdoor as their origin.
2. **Given** a Glassdoor listing was already ingested in a previous run, **When** the same
   run is repeated, **Then** the listing is recognized as already known and is not
   duplicated in the feed.
3. **Given** Glassdoor listings are in the feed, **When** the user filters or sorts the feed
   by source, **Then** Glassdoor appears as a selectable source alongside Djinni, DOU,
   Indeed, and RemoteOK.

---

### User Story 2 - Manage the Glassdoor source like any other source (Priority: P2)

An operator opens the Sources screen and manages Glassdoor exactly as they manage the other
sources: enable/disable it, edit its configuration, run it on demand, test its health, and
see its last run status and result counts.

**Why this priority**: Parity in the management surface is what makes the source usable and
debuggable day to day, but the feature still delivers value if the source is only runnable
on its normal schedule.

**Independent Test**: From the Sources screen, save a Glassdoor subscription, toggle
Glassdoor off and on, run a health test, trigger a manual run, and verify run history and
counts update — all without touching other sources' behavior.

**Acceptance Scenarios**:

1. **Given** the Sources screen is open, **When** the operator views the source list,
   **Then** Glassdoor is listed with its enabled state, health state, and last run summary.
2. **Given** the operator saves a Glassdoor subscription (a Glassdoor job-search URL or a
   job title plus location), **When** they save it, **Then** it is accepted and becomes the
   configuration the next Glassdoor run executes against; an input that is not a
   recognizable Glassdoor subscription is rejected with a stated reason.
3. **Given** Glassdoor is enabled, **When** the operator triggers a health test, **Then**
   the system reports whether Glassdoor is currently reachable and usable, with a
   human-readable reason on failure (including when Glassdoor is blocking automated access).
4. **Given** Glassdoor is disabled, **When** a scheduled ingestion cycle runs, **Then** no
   Glassdoor requests are made and no Glassdoor listings are added.

---

### User Story 3 - Enrich Glassdoor listings with full posting detail (Priority: P3)

A user opens a Glassdoor listing from the feed and sees the full job description, salary
estimate, and company rating — not just the summary fields returned by the initial listing
fetch — so that matching, scoring, and generated application materials are grounded in the
complete posting.

**Why this priority**: Grounded generation depends on the full description, but the feed is
already useful with summary-level data, so this can follow the initial ingestion slice.

**Independent Test**: Ingest one Glassdoor listing, run enrichment for it, and confirm the
stored description, salary estimate, and posting date are captured in full.

**Acceptance Scenarios**:

1. **Given** a Glassdoor listing was ingested with summary data, **When** enrichment runs
   for that listing, **Then** the stored posting contains the full description text and a
   resolved posting date, plus salary estimate and company rating when Glassdoor publishes
   them.
2. **Given** a Glassdoor listing whose posting is no longer available, **When** enrichment
   runs for it, **Then** the listing is marked as unavailable and the summary data already
   captured is preserved rather than discarded.

---

### Edge Cases

- **Glassdoor blocks or throttles the request** (rate limiting, access denial, bot
  challenge): the run ends with a recorded failure and a human-readable reason distinct from
  "no results"; already-ingested listings from earlier in the run are kept, and the source's
  health state reflects the failure.
- **Zero results** for a valid configuration: the run completes successfully with a count of
  zero rather than being reported as an error.
- **Response shape changes upstream** so nothing parses: the run reports a failure with a
  distinguishable reason (results were returned but none could be interpreted), so the cause
  is not confused with "no matching jobs" or "blocked".
- **Same job posted on Glassdoor and on another source**: the feed shows a single entry per
  distinct posting per existing deduplication behavior; the Glassdoor origin is retained on
  whichever record is kept.
- **Listing has an estimated (not employer-stated) salary range**: the estimate is captured
  and marked as an estimate rather than treated as an employer-confirmed figure.
- **Listing requires a Glassdoor account to view full detail**: the listing is still
  ingested from publicly visible summary data; enrichment marks the detail as unavailable
  rather than attempting to authenticate.
- **Long-running run interrupted** (cancellation, shutdown, timeout): listings gathered
  before the interruption are retained and the run is recorded as partial.
- **Saved subscription is not a recognizable Glassdoor search URL or title/location pair**:
  saving it is rejected with a human-readable reason at configuration time, not silently at
  run time.
- **Upstream response exceeds the expected listing count for a single fetch**: the run keeps
  all listings returned in that fetch, records how many it collected, and reports success.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST make Glassdoor available as a first-class job source, selectable
  and attributable in the same places Djinni, DOU, Indeed, and RemoteOK are (feed source
  attribution, source filters, saved-search source selection).
- **FR-002**: System MUST retrieve Glassdoor listings for a user-defined search configuration
  and add them to the job feed.
- **FR-003**: Each ingested Glassdoor listing MUST carry, when the source publishes it:
  title, company, location text, remote indicator, salary range (marked estimated or
  employer-stated), posting URL, description, posting date, and a stable per-listing
  identifier.
- **FR-004**: System MUST recognize previously ingested Glassdoor listings across runs and
  not create duplicate feed entries for them.
- **FR-005**: System MUST allow the Glassdoor source to be enabled and disabled, and MUST
  make no Glassdoor requests while it is disabled.
- **FR-006**: System MUST support an on-demand run and an on-demand health check for the
  Glassdoor source, reporting outcome and a human-readable reason on failure.
- **FR-007**: System MUST record each Glassdoor run's outcome (succeeded, failed, or
  partial), the number of listings found, and the number newly added.
- **FR-008**: System MUST retain listings already collected when a run is interrupted or
  fails partway, and MUST record the run as partial rather than discarding results.
- **FR-009**: System MUST support retrieving the full detail of an individual Glassdoor
  listing so that description, salary estimate, and posting date can be completed after
  initial ingestion, when the initial fetch did not already include them.
- **FR-010**: System MUST pace its Glassdoor requests so that a single run does not issue
  requests faster than one every 500 milliseconds, and MUST bound the number of requests a
  single run can make.
- **FR-011**: System MUST distinguish, in run outcomes, between "source returned no matching
  listings", "source returned content that could not be interpreted", and "source blocked
  the request".
- **FR-012**: Ingested Glassdoor listings MUST flow through the same downstream matching,
  scoring, and application-material generation paths as listings from existing sources, with
  no Glassdoor-specific exception.
- **FR-013**: System MUST NOT submit applications, send messages, or take any action on a
  Glassdoor listing on the user's behalf, and MUST NOT authenticate with a Glassdoor account;
  Glassdoor integration is discovery-only against publicly visible listings.
- **FR-014**: System MUST accept a Glassdoor search configuration as an operator-saved
  subscription (a Glassdoor job-search URL, or a job title plus location), managed through
  the same subscription flow already used for DOU, Djinni, Indeed, and RemoteOK.
- **FR-015**: System MUST reject a saved subscription that is not a recognizable Glassdoor
  configuration, with a human-readable reason, rather than saving it and failing at run time.
- **FR-016**: The Glassdoor source MUST expose its own identity in run records, source
  attribution, and health status, distinct from any other source.
- **FR-017**: System MUST identify its requests to Glassdoor with a descriptive client
  identifier, consistent with the conservative-access posture used for other sources.
- **FR-018**: System MUST treat a Glassdoor block/challenge response as a distinct, reported
  failure mode rather than retrying aggressively or attempting to bypass the block.

### Key Entities *(include if data involved)*

- **Job Source (Glassdoor)**: the registered, per-source configuration record — enabled
  state, configuration values, health state, and last run summary.
- **Source Subscription**: a saved Glassdoor search configuration (search URL, or job title
  plus location) that a run executes against; one source may have several.
- **Normalized Job Listing**: a Glassdoor posting converted into the product's common job
  shape (title, company, location, remote flag, salary range with estimate flag, URL,
  description, posting date, source origin, stable identifier).
- **Source Run**: a record of one Glassdoor execution — start/end, outcome, listings found,
  listings newly added, failure reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can save a Glassdoor subscription, enable the source, and see
  Glassdoor listings in the job feed within 5 minutes, with no code change or redeploy.
- **SC-002**: A single Glassdoor run against a typical subscription returns at least 20
  distinct listings, or all available listings if fewer than 20 exist.
- **SC-003**: Re-running the same Glassdoor configuration immediately adds zero new feed
  entries — 100% of already-known listings are recognized as duplicates.
- **SC-004**: At least 95% of ingested Glassdoor listings have a non-empty title, company,
  and openable posting URL.
- **SC-005**: When Glassdoor is unreachable, blocks the run, or challenges it, the source's
  status shows unhealthy with a human-readable reason within one run cycle, and no other
  source's runs are affected.
- **SC-006**: After enrichment, at least 90% of Glassdoor listings have a description longer
  than the summary captured at ingestion time.
- **SC-007**: Adding Glassdoor does not increase the median end-to-end ingestion cycle time
  for the existing sources by more than 10%.
- **SC-008**: 100% of saved subscriptions that are not valid Glassdoor configurations are
  rejected at save time with a stated reason, and none reach a run.

## Assumptions

- Glassdoor is added as an additional source; no existing source (Djinni, DOU, Indeed,
  RemoteOK, or any other) is removed, disabled, or changed in behavior by this feature.
- Glassdoor listings reuse the existing job-feed, deduplication, matching, scoring, and
  generation pipelines unchanged — this feature adds a source, not a new pipeline.
- No Glassdoor user account, login, or authenticated session is required or stored; only
  publicly visible listings and their publicly visible summary/detail pages are in scope.
- Glassdoor is known to actively rate-limit and challenge automated access more aggressively
  than the existing sources; this feature respects that posture via conservative request
  pacing, a descriptive client identifier, and a bounded number of requests per run, and
  treats blocked/challenged runs as an expected, reportable failure mode rather than
  something to defeat — aggressive crawling and anti-bot evasion are explicitly out of
  scope.
- Salary and company-rating data are captured opportunistically when Glassdoor publishes
  them for a listing; their absence does not block ingestion of that listing.
- Operators configure Glassdoor the same way they configure DOU, Djinni, Indeed, and
  RemoteOK — saving a subscription through the existing Sources screen flow.
- Only listing discovery and detail retrieval are in scope; applying, messaging employers,
  and syncing Glassdoor application status are out of scope, per the product's no-auto-apply
  rule.
- Seed/demo data and management-screen coverage for Glassdoor follow the same conventions as
  existing sources.
- Upstream response shape and anti-bot measures are expected to change over time; the source
  is treated as best-effort, and its failures must not fail the overall ingestion cycle.
