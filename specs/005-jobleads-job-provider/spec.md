# Feature Specification: JobLeads Job Source

**Feature Branch**: `005-jobleads-job-provider`

**Created**: 2026-07-24

**Status**: Draft

**Input**: User description: "lets JobLeads job provider"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover JobLeads listings alongside existing sources (Priority: P1)

A job seeker who already receives listings from Djinni, DOU, Indeed, and RemoteOK wants
JobLeads listings to appear in the same job feed, deduplicated and scored the same way,
without having to open JobLeads manually.

**Why this priority**: This is the core value — JobLeads adds curated/hidden-market listings
the other sources may not carry. Without listings flowing into the feed, nothing else in this
feature matters.

**Independent Test**: Enable the JobLeads source, trigger a run, and confirm new job records
appear in the feed tagged with JobLeads as their origin, each with title, company, location,
remote flag, posting URL, and description.

**Acceptance Scenarios**:

1. **Given** the JobLeads source is enabled and a search configuration exists, **When** a
   source run is triggered, **Then** JobLeads listings matching that configuration are added
   to the job feed and attributed to JobLeads as their origin.
2. **Given** a JobLeads listing was already ingested in a previous run, **When** the same run
   is repeated, **Then** the listing is recognized as already known and is not duplicated in
   the feed.
3. **Given** JobLeads listings are in the feed, **When** the user filters or sorts the feed by
   source, **Then** JobLeads appears as a selectable source alongside Djinni, DOU, Indeed, and
   RemoteOK.

---

### User Story 2 - Manage the JobLeads source like any other source (Priority: P2)

An operator opens the Sources screen and manages JobLeads exactly as they manage the other
sources: enable/disable it, edit its configuration, run it on demand, test its health, and
see its last run status and result counts.

**Why this priority**: Parity in the management surface is what makes the source usable and
debuggable day to day, but the feature still delivers value if the source is only runnable on
its normal schedule.

**Independent Test**: From the Sources screen, save a JobLeads subscription, toggle JobLeads
off and on, run a health test, trigger a manual run, and verify run history and counts
update — all without touching other sources' behavior.

**Acceptance Scenarios**:

1. **Given** the Sources screen is open, **When** the operator views the source list, **Then**
   JobLeads is listed with its enabled state, health state, and last run summary.
2. **Given** the operator saves a JobLeads subscription (a saved search or category
   configuration), **When** they save it, **Then** it is accepted and becomes the
   configuration the next JobLeads run executes against; an input that is not a recognizable
   JobLeads subscription is rejected with a stated reason.
3. **Given** JobLeads is enabled, **When** the operator triggers a health test, **Then** the
   system reports whether JobLeads is currently reachable and usable, with a human-readable
   reason on failure — including when stored access credentials have expired or been revoked.
4. **Given** JobLeads is disabled, **When** a scheduled ingestion cycle runs, **Then** no
   JobLeads requests are made and no JobLeads listings are added.

---

### User Story 3 - Enrich JobLeads listings with full posting detail (Priority: P3)

A user opens a JobLeads listing from the feed and sees the full job description and
qualifications — not just the summary fields returned by the initial listing fetch — so that
matching, scoring, and generated application materials are grounded in the complete posting.

**Why this priority**: Grounded generation depends on the full description, but the feed is
already useful with summary-level data, so this can follow the initial ingestion slice.

**Independent Test**: Ingest one JobLeads listing, run enrichment for it, and confirm the
stored description and qualifications are captured in full, with posting date resolved.

**Acceptance Scenarios**:

1. **Given** a JobLeads listing was ingested with summary data, **When** enrichment runs for
   that listing, **Then** the stored posting contains the full description text and a
   resolved posting date.
2. **Given** a JobLeads listing whose posting is no longer available, **When** enrichment runs
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
- **Same job posted on JobLeads and on another source**: the feed shows a single entry per
  distinct posting per existing deduplication behavior; the JobLeads origin is retained on
  whichever record is kept.
- **Stored account credentials expire or are revoked**: the run fails with a distinguishable
  reason ("authentication required"), the source's health state reflects it, and no further
  runs are attempted until access is restored.
- **Listing has no salary or no explicit location**: the listing is still ingested with those
  fields empty rather than being dropped.
- **Long-running run interrupted** (cancellation, shutdown, timeout): listings gathered before
  the interruption are retained and the run is recorded as partial.
- **Saved subscription is not a recognizable JobLeads configuration** (wrong site, malformed,
  or unknown search): saving it is rejected with a human-readable reason at configuration
  time, not silently at run time.
- **Upstream response exceeds the expected listing count for a single fetch**: the run keeps
  all listings returned in that fetch, records how many it collected, and reports success.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST make JobLeads available as a first-class job source, selectable and
  attributable in the same places Djinni, DOU, Indeed, and RemoteOK are (feed source
  attribution, source filters, saved-search source selection).
- **FR-002**: System MUST retrieve JobLeads listings for a user-defined search configuration
  and add them to the job feed.
- **FR-003**: Each ingested JobLeads listing MUST carry, when the source publishes it: title,
  company, location text, remote indicator, raw salary text, posting URL, description, and a
  stable per-listing identifier.
- **FR-004**: System MUST recognize previously ingested JobLeads listings across runs and not
  create duplicate feed entries for them.
- **FR-005**: System MUST allow the JobLeads source to be enabled and disabled, and MUST make
  no JobLeads requests while it is disabled.
- **FR-006**: System MUST support an on-demand run and an on-demand health check for the
  JobLeads source, reporting outcome and a human-readable reason on failure.
- **FR-007**: System MUST record each JobLeads run's outcome (succeeded, failed, or partial),
  the number of listings found, and the number newly added.
- **FR-008**: System MUST retain listings already collected when a run is interrupted or fails
  partway, and MUST record the run as partial rather than discarding results.
- **FR-009**: System MUST support retrieving the full detail of an individual JobLeads listing
  so that description and posting date can be completed after initial ingestion, when the
  initial fetch did not already include them.
- **FR-010**: System MUST pace its JobLeads requests so that a single run does not issue
  requests faster than one every 500 milliseconds, and MUST bound the number of requests a
  single run can make.
- **FR-011**: System MUST distinguish, in run outcomes, between "source returned no matching
  listings", "source returned content that could not be interpreted", and "access to the
  source is not authorized" (expired/revoked credentials).
- **FR-012**: Ingested JobLeads listings MUST flow through the same downstream matching,
  scoring, and application-material generation paths as listings from existing sources, with
  no JobLeads-specific exception.
- **FR-013**: System MUST NOT submit applications, send messages, or take any action on a
  JobLeads listing on the user's behalf; JobLeads integration is discovery-only.
- **FR-014**: System MUST accept a JobLeads search configuration as an operator-saved
  subscription, managed through the same subscription flow already used for DOU, Djinni,
  Indeed, and RemoteOK.
- **FR-015**: System MUST reject a saved subscription that is not a recognizable JobLeads
  configuration, with a human-readable reason, rather than saving it and failing at run time.
- **FR-016**: The JobLeads source MUST expose its own identity in run records, source
  attribution, and health status, distinct from any other source.
- **FR-017**: System MUST identify its requests to JobLeads with a descriptive client
  identifier, consistent with the conservative-access posture used for other sources.
- **FR-018**: System MUST store any JobLeads account credentials the operator supplies
  securely (not in plain text, not exposed in logs or the UI after entry) and MUST use them
  only to retrieve listings on the operator's behalf.

### Key Entities *(include if data involved)*

- **Job Source (JobLeads)**: the registered, per-source configuration record — enabled state,
  configuration values, health state, and last run summary.
- **Source Subscription**: a saved JobLeads search configuration that a run executes against;
  one source may have several.
- **Source Credentials**: the operator-supplied JobLeads account credentials used to access
  listings, stored securely and associated with the JobLeads source.
- **Normalized Job Listing**: a JobLeads posting converted into the product's common job shape
  (title, company, location, remote flag, salary text, URL, description, posting date, source
  origin, stable identifier).
- **Source Run**: a record of one JobLeads execution — start/end, outcome, listings found,
  listings newly added, failure reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can save a JobLeads subscription, enable the source, and see
  JobLeads listings in the job feed within 5 minutes, with no code change or redeploy.
- **SC-002**: A single JobLeads run against a typical subscription returns at least 20 distinct
  listings, or all available listings if fewer than 20 exist.
- **SC-003**: Re-running the same JobLeads configuration immediately adds zero new feed
  entries — 100% of already-known listings are recognized as duplicates.
- **SC-004**: At least 95% of ingested JobLeads listings have a non-empty title, company, and
  openable posting URL.
- **SC-005**: When JobLeads is unreachable, blocks the run, or reports invalid credentials, the
  source's status shows unhealthy with a human-readable reason within one run cycle, and no
  other source's runs are affected.
- **SC-006**: After enrichment, at least 90% of JobLeads listings have a description longer
  than the summary captured at ingestion time.
- **SC-007**: Adding JobLeads does not increase the median end-to-end ingestion cycle time for
  the existing sources by more than 10%.
- **SC-008**: 100% of saved subscriptions that are not valid JobLeads configurations are
  rejected at save time with a stated reason, and none reach a run.

## Assumptions

- JobLeads is added as an additional source; no existing source (Djinni, DOU, Indeed,
  RemoteOK, or any other) is removed, disabled, or changed in behavior by this feature.
- JobLeads listings reuse the existing job-feed, deduplication, matching, scoring, and
  generation pipelines unchanged — this feature adds a source, not a new pipeline.
- JobLeads requires an operator-supplied account to access listings (unlike public-listing
  sources such as RemoteOK); credentials are entered once through the Sources screen and
  stored securely rather than re-entered per run.
- JobLeads's terms and rate limits are respected via conservative request pacing, a
  descriptive client identifier, and a bounded number of requests per run; aggressive crawling
  and anti-bot evasion are out of scope.
- Operators configure JobLeads the same way they configure DOU, Djinni, Indeed, and RemoteOK —
  saving a subscription through the existing Sources screen flow, plus one-time account
  credential entry.
- Only listing discovery and detail retrieval are in scope; applying, messaging employers, and
  syncing JobLeads application status are out of scope, per the product's no-auto-apply rule.
- Seed/demo data and management-screen coverage for JobLeads follow the same conventions as
  existing sources.
- Upstream response shape is expected to change over time; the source is treated as
  best-effort, and its failures must not fail the overall ingestion cycle.
