# Feature Specification: RemoteOK Job Source

**Feature Branch**: `003-remoteok-job-provider`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "lets add new remoteok job provider"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover RemoteOK listings alongside existing sources (Priority: P1)

A job seeker who already receives listings from Djinni, DOU, and Indeed wants RemoteOK
listings to appear in the same job feed, deduplicated and scored the same way, without
having to open RemoteOK manually.

**Why this priority**: This is the core value — RemoteOK is a dedicated remote-jobs board
and adds listings the other sources may not carry. Without listings flowing into the feed,
nothing else in this feature matters.

**Independent Test**: Enable the RemoteOK source, trigger a run, and confirm new job records
appear in the feed tagged with RemoteOK as their origin, each with title, company, location,
remote flag, posting URL, and description.

**Acceptance Scenarios**:

1. **Given** the RemoteOK source is enabled and a search configuration exists, **When** a
   source run is triggered, **Then** RemoteOK listings matching that configuration are added
   to the job feed and attributed to RemoteOK as their origin.
2. **Given** a RemoteOK listing was already ingested in a previous run, **When** the same
   run is repeated, **Then** the listing is recognized as already known and is not
   duplicated in the feed.
3. **Given** RemoteOK listings are in the feed, **When** the user filters or sorts the feed
   by source, **Then** RemoteOK appears as a selectable source alongside Djinni, DOU, and
   Indeed.

---

### User Story 2 - Manage the RemoteOK source like any other source (Priority: P2)

An operator opens the Sources screen and manages RemoteOK exactly as they manage the other
sources: enable/disable it, edit its configuration, run it on demand, test its health, and
see its last run status and result counts.

**Why this priority**: Parity in the management surface is what makes the source usable and
debuggable day to day, but the feature still delivers value if the source is only runnable
on its normal schedule.

**Independent Test**: From the Sources screen, save a RemoteOK subscription, toggle RemoteOK
off and on, run a health test, trigger a manual run, and verify run history and counts
update — all without touching other sources' behavior.

**Acceptance Scenarios**:

1. **Given** the Sources screen is open, **When** the operator views the source list,
   **Then** RemoteOK is listed with its enabled state, health state, and last run summary.
2. **Given** the operator saves a RemoteOK subscription (a RemoteOK listing page URL or a
   tag/category), **When** they save it, **Then** it is accepted and becomes the
   configuration the next RemoteOK run executes against; an input that is not a recognizable
   RemoteOK subscription is rejected with a stated reason.
3. **Given** RemoteOK is enabled, **When** the operator triggers a health test, **Then** the
   system reports whether RemoteOK is currently reachable and usable, with a human-readable
   reason on failure.
4. **Given** RemoteOK is disabled, **When** a scheduled ingestion cycle runs, **Then** no
   RemoteOK requests are made and no RemoteOK listings are added.

---

### User Story 3 - Enrich RemoteOK listings with full posting detail (Priority: P3)

A user opens a RemoteOK listing from the feed and sees the full job description, salary, and
tags — not just the summary fields returned by the initial listing fetch — so that matching,
scoring, and generated application materials are grounded in the complete posting.

**Why this priority**: Grounded generation depends on the full description, but the feed is
already useful with summary-level data, so this can follow the initial ingestion slice.

**Independent Test**: Ingest one RemoteOK listing, run enrichment for it, and confirm the
stored description, tags, and salary (when published) are captured in full, with posting
date resolved.

**Acceptance Scenarios**:

1. **Given** a RemoteOK listing was ingested with summary data, **When** enrichment runs for
   that listing, **Then** the stored posting contains the full description text, tags, and a
   resolved posting date.
2. **Given** a RemoteOK listing whose posting is no longer available, **When** enrichment
   runs for it, **Then** the listing is marked as unavailable and the summary data already
   captured is preserved rather than discarded.

---

### Edge Cases

- **Source blocks or throttles the request** (rate limiting, access denial): the run ends
  with a recorded failure and a human-readable reason; already-ingested listings from
  earlier in the run are kept, and the source's health state reflects the failure.
- **Zero results** for a valid configuration: the run completes successfully with a count of
  zero rather than being reported as an error.
- **Response shape changes upstream** so nothing parses: the run reports a failure with a
  distinguishable reason (results were returned but none could be interpreted), so the cause
  is not confused with "no matching jobs".
- **Same job posted on RemoteOK and on another source**: the feed shows a single entry per
  distinct posting per existing deduplication behavior; the RemoteOK origin is retained on
  whichever record is kept.
- **Listing has no salary or no explicit location** (common on RemoteOK, which is
  remote-only): the listing is still ingested with those fields empty rather than being
  dropped, and its remote flag is always set.
- **Sponsored/featured listings mixed with organic ones**: both are ingested identically;
  sponsorship status is not used to include or exclude a listing.
- **Long-running run interrupted** (cancellation, shutdown, timeout): listings gathered
  before the interruption are retained and the run is recorded as partial.
- **Saved subscription is not a recognizable RemoteOK URL or tag** (wrong site, malformed,
  or unknown tag): saving it is rejected with a human-readable reason at configuration time,
  not silently at run time.
- **Upstream response exceeds the expected listing count for a single fetch**: the run keeps
  all listings returned in that fetch, records how many it collected, and reports success.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST make RemoteOK available as a first-class job source, selectable
  and attributable in the same places Djinni, DOU, and Indeed are (feed source attribution,
  source filters, saved-search source selection).
- **FR-002**: System MUST retrieve RemoteOK listings for a user-defined search configuration
  and add them to the job feed.
- **FR-003**: Each ingested RemoteOK listing MUST carry, when the source publishes it: title,
  company, location text, remote indicator (always true), raw salary text, posting URL,
  description, tags, posting date, and a stable per-listing identifier.
- **FR-004**: System MUST recognize previously ingested RemoteOK listings across runs and
  not create duplicate feed entries for them.
- **FR-005**: System MUST allow the RemoteOK source to be enabled and disabled, and MUST
  make no RemoteOK requests while it is disabled.
- **FR-006**: System MUST support an on-demand run and an on-demand health check for the
  RemoteOK source, reporting outcome and a human-readable reason on failure.
- **FR-007**: System MUST record each RemoteOK run's outcome (succeeded, failed, or
  partial), the number of listings found, and the number newly added.
- **FR-008**: System MUST retain listings already collected when a run is interrupted or
  fails partway, and MUST record the run as partial rather than discarding results.
- **FR-009**: System MUST support retrieving the full detail of an individual RemoteOK
  listing so that description, tags, and posting date can be completed after initial
  ingestion, when the initial fetch did not already include them.
- **FR-010**: System MUST pace its RemoteOK requests so that a single run does not issue
  requests faster than one every 500 milliseconds, and MUST bound the number of requests a
  single run can make.
- **FR-011**: System MUST distinguish, in run outcomes, between "source returned no
  matching listings" and "source returned content that could not be interpreted".
- **FR-012**: Ingested RemoteOK listings MUST flow through the same downstream matching,
  scoring, and application-material generation paths as listings from existing sources,
  with no RemoteOK-specific exception.
- **FR-013**: System MUST NOT submit applications, send messages, or take any action on a
  RemoteOK listing on the user's behalf; RemoteOK integration is discovery-only.
- **FR-014**: System MUST accept a RemoteOK search configuration as an operator-saved
  subscription (a RemoteOK category/tag or listing-page URL), managed through the same
  subscription flow already used for DOU, Djinni, and Indeed.
- **FR-015**: System MUST reject a saved subscription that is not a recognizable RemoteOK
  configuration, with a human-readable reason, rather than saving it and failing at run
  time.
- **FR-016**: The RemoteOK source MUST expose its own identity in run records, source
  attribution, and health status, distinct from any other source.
- **FR-017**: System MUST identify its requests to RemoteOK with a descriptive client
  identifier, consistent with the conservative-access posture used for other sources.

### Key Entities *(include if data involved)*

- **Job Source (RemoteOK)**: the registered, per-source configuration record — enabled
  state, configuration values, health state, and last run summary.
- **Source Subscription**: a saved RemoteOK search configuration (category/tag or listing
  URL) that a run executes against; one source may have several.
- **Normalized Job Listing**: a RemoteOK posting converted into the product's common job
  shape (title, company, location, remote flag, salary text, URL, description, tags,
  posting date, source origin, stable identifier).
- **Source Run**: a record of one RemoteOK execution — start/end, outcome, listings found,
  listings newly added, failure reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can save a RemoteOK subscription, enable the source, and see
  RemoteOK listings in the job feed within 5 minutes, with no code change or redeploy.
- **SC-002**: A single RemoteOK run against a typical subscription returns at least 20
  distinct listings, or all available listings if fewer than 20 exist.
- **SC-003**: Re-running the same RemoteOK configuration immediately adds zero new feed
  entries — 100% of already-known listings are recognized as duplicates.
- **SC-004**: At least 95% of ingested RemoteOK listings have a non-empty title, company,
  and openable posting URL.
- **SC-005**: When RemoteOK is unreachable or blocks the run, the source's status shows
  unhealthy with a human-readable reason within one run cycle, and no other source's runs
  are affected.
- **SC-006**: After enrichment, at least 90% of RemoteOK listings have a description longer
  than the summary captured at ingestion time.
- **SC-007**: Adding RemoteOK does not increase the median end-to-end ingestion cycle time
  for the existing sources by more than 10%.
- **SC-008**: 100% of saved subscriptions that are not valid RemoteOK configurations are
  rejected at save time with a stated reason, and none reach a run.

## Assumptions

- RemoteOK is added as an additional source; no existing source (Djinni, DOU, Indeed, or any
  other) is removed, disabled, or changed in behavior by this feature.
- RemoteOK listings reuse the existing job-feed, deduplication, matching, scoring, and
  generation pipelines unchanged — this feature adds a source, not a new pipeline.
- No RemoteOK user account, login, or authenticated session is required or stored; only
  publicly visible listings are in scope.
- RemoteOK's terms and rate limits are respected via conservative request pacing, a
  descriptive client identifier, and a bounded number of requests per run; aggressive
  crawling and anti-bot evasion are out of scope.
- All RemoteOK listings are remote by nature of the source; no separate remote-detection
  logic is needed for RemoteOK the way it may be for other sources.
- Operators configure RemoteOK the same way they configure DOU, Djinni, and Indeed — saving
  a subscription through the existing Sources screen flow.
- Only listing discovery and detail retrieval are in scope; applying, messaging employers,
  and syncing RemoteOK application status are out of scope, per the product's no-auto-apply
  rule.
- Seed/demo data and management-screen coverage for RemoteOK follow the same conventions as
  existing sources.
- Upstream response shape is expected to change over time; the source is treated as
  best-effort, and its failures must not fail the overall ingestion cycle.
