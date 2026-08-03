> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/job-sources.md`](../../domains/job-sources.md) — read that first.

---
# Feature Specification: Employer ATS Board Sources

**Feature Branch**: `013-ats-board-sources`

**Created**: 2026-07-25

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "Free job discovery that avoids bot detection by reading employer
applicant-tracking-system job boards directly instead of scraping protected aggregators."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover listings straight from employer job boards (Priority: P1)

A job seeker whose feed depends on aggregators that intermittently block or challenge automated
access wants listings to arrive from the employers' own hosted job boards instead. Those boards
publish their openings in a stable, machine-readable form for anyone to read, with no login, no
challenge page, and no request budget to negotiate — so listings arrive reliably, and the same
job usually arrives *earlier* than it does through an aggregator.

**Why this priority**: This is the whole point of the feature. The most common jobs the user
misses today are jobs whose aggregator page could not be retrieved, even though the employer
publishes the same posting openly. Without listings flowing in from employer boards, nothing
else here matters.

**Independent Test**: Register one employer board for each supported board vendor, trigger a
source run, and confirm new job records appear in the feed attributed to that employer's board,
each carrying title, company, location, remote flag, apply URL, and full description.

**Acceptance Scenarios**:

1. **Given** an employer board is registered and its source is enabled, **When** a source run is
   triggered, **Then** every open posting on that board is added to the feed with title, company,
   location, remote flag, apply URL, and the full posting description.
2. **Given** a posting was ingested in a previous run, **When** the run repeats, **Then** the
   posting is recognized as already known and is not duplicated.
3. **Given** a posting was open in the previous run and has since been taken down, **When** the
   run repeats, **Then** the run completes normally and the missing posting does not cause an
   error.
4. **Given** a run against an employer board, **When** the run completes, **Then** it required no
   credentials, no stored session, and no challenge-solving step of any kind.
5. **Given** a board's postings arrive with the full description already present, **When** the
   listings are ingested, **Then** they are scored without waiting for a separate per-listing
   enrichment pass.

---

### User Story 2 - Build the employer roster without manual research (Priority: P1)

The value of this source family scales with the number of employer boards registered, so the
system proposes candidates instead of making the user hunt for them. Many listings already in
the feed link to an employer board, so the system can recognise those links, work out which
employer and which board vendor they belong to, and offer the board as a candidate the user can
accept or reject.

**Why this priority**: Equal to US1 because a working reader with an empty roster delivers
nothing, and hand-registering employers one at a time is the kind of chore that stops the
feature from ever being used at scale.

**Independent Test**: With existing listings in the feed that link to employer boards, run
candidate discovery and confirm the proposed candidates name the right employer and board
vendor, and that accepting one causes that board's postings to appear on the next run.

**Acceptance Scenarios**:

1. **Given** listings in the feed whose apply URLs point at supported board vendors, **When**
   candidate discovery runs, **Then** each distinct employer board found is offered as a
   candidate with its employer name and board vendor.
2. **Given** a proposed candidate, **When** the user accepts it, **Then** it joins the roster and
   its postings are ingested on the next run.
3. **Given** a proposed candidate, **When** the user rejects it, **Then** it is not proposed
   again.
4. **Given** a board URL the user pastes in directly, **When** it is saved, **Then** the system
   identifies its vendor and employer, confirms the board is readable, and adds it to the roster
   — or explains why it could not.
5. **Given** a candidate whose board turns out to be unreadable or empty, **When** discovery
   validates it, **Then** it is not silently added and the reason is shown.

---

### User Story 3 - Recognise the same job arriving from two places (Priority: P2)

A posting the user already saw via an aggregator will often arrive again from the employer's own
board. The user must see one job, not two — and the copy they keep should be the one that links
to the employer's real application page, since that is where they will actually apply.

**Why this priority**: Without it, the feature's success actively degrades the feed by filling
it with duplicates. It is P2 only because a roster of employers not otherwise covered still
delivers clean value on day one.

**Independent Test**: Ingest a posting from an aggregator, then ingest the same posting from the
employer's board, and confirm the feed shows one job whose apply URL points at the employer
board and which lists both origins.

**Acceptance Scenarios**:

1. **Given** the same posting is available from an aggregator and from the employer's board,
   **When** both are ingested, **Then** the feed contains one job, not two.
2. **Given** such a merge, **When** the user opens the job, **Then** its apply URL is the
   employer board's, and both origins are visible on the record.
3. **Given** the user has already acted on the aggregator copy — saved it, scored it, or moved it
   on the board — **When** the employer-board copy merges into it, **Then** that history is
   preserved.
4. **Given** two postings that merely resemble each other (same title at different employers, or
   two genuinely distinct openings for one role), **When** both are ingested, **Then** they are
   kept separate.

---

### User Story 4 - Manage board sources like every other source (Priority: P2)

An operator manages employer board sources from the Sources screen the same way they manage the
existing sources: enable and disable them, run one on demand, check its health, and read its
last run status and counts. A roster of many employers must stay legible — the operator needs to
see which employers are registered, which are producing listings, and which have gone stale.

**Why this priority**: Parity is what makes the source family debuggable day to day, but the
feature still delivers value running on its normal schedule.

**Independent Test**: From the Sources screen, enable a board vendor source, run it on demand,
watch its health check pass, and read its resulting counts — then disable it and confirm the next
scheduled run skips it.

**Acceptance Scenarios**:

1. **Given** the Sources screen, **When** the operator views it, **Then** each supported board
   vendor appears as a source that can be enabled, disabled, run on demand, and health-checked.
2. **Given** a completed run, **When** the operator reads its record, **Then** they see how many
   employers were read, how many postings were found, how many were new, and how many employers
   failed.
3. **Given** one employer's board fails during a run, **When** the run continues, **Then** the
   remaining employers are still read and the failure is attributed to that employer alone.
4. **Given** an employer whose board has returned nothing for several consecutive runs, **When**
   the operator views the roster, **Then** that employer is flagged as stale so it can be
   removed.

---

### Edge Cases

- An employer board is removed, renamed, or made private: that employer's runs fail with a clear
  reason, other employers are unaffected, and repeated failure flags it as stale rather than
  retrying forever.
- A board's published shape changes: the run reports a parse failure for that employer instead of
  ingesting empty or half-built records.
- A board publishes hundreds of openings, or the roster grows to hundreds of employers: the run
  stays bounded and paced, and a partially completed run keeps what it already ingested.
- Two employers use the same board identifier on different vendors, or one employer runs boards
  on two vendors at once: both are registered without collision, and their postings still merge
  where they are the same job.
- A posting carries no location, no description, or a description that is empty boilerplate: it
  is ingested with the fields that exist, or skipped with a reason if it is unusable.
- A posting is an "evergreen" or speculative req that never closes: it is ingested once and not
  resurfaced as new on every subsequent run.
- A board is reachable but returns an authorization or rate-limit response: it is reported as
  that, distinctly from "no openings".
- The user pastes a URL for a vendor the system does not support: it is rejected with a message
  naming the supported vendors, not silently accepted.
- Candidate discovery finds an employer already in the roster: it is not offered again as new.

## Requirements *(mandatory)*

### Functional Requirements

**Reading employer boards**

- **FR-001**: System MUST read open postings from employer-hosted job boards on the supported
  board vendors without credentials, stored sessions, or challenge-solving of any kind.
- **FR-002**: System MUST support at minimum these board vendors at launch: Greenhouse, Lever,
  Ashby, Workable, and SmartRecruiters.
- **FR-003**: System MUST normalize each posting into the same job shape used by every existing
  source, populating title, company, location, remote flag, apply URL, description, and posting
  date where the board publishes them.
- **FR-004**: System MUST treat postings that arrive with a full description as needing no
  separate per-listing enrichment pass, so they are scored on first ingest.
- **FR-005**: System MUST add a new board vendor by adding one reader for that vendor, without
  changing shared ingestion, scoring, or storage behavior.
- **FR-006**: System MUST pace requests to each board host and MUST NOT exceed the pacing already
  enforced for third-party hosts elsewhere in the system.
- **FR-007**: System MUST cap the work of a single run — a maximum number of employers read and a
  maximum number of postings per employer — so one run cannot grow unbounded.

**Employer roster**

- **FR-008**: System MUST maintain a roster of registered employer boards, each identified by its
  board vendor and the employer's identifier on that vendor.
- **FR-009**: System MUST propose roster candidates by recognising supported board links among
  the apply URLs of listings already ingested, and MUST derive each candidate's employer and
  vendor from that link.
- **FR-010**: Users MUST be able to accept or reject a proposed candidate, and a rejected
  candidate MUST NOT be proposed again.
- **FR-011**: Users MUST be able to register an employer board by pasting its URL, and the system
  MUST validate that the board is readable before adding it.
- **FR-012**: Users MUST be able to remove an employer from the roster, and removal MUST NOT
  delete jobs already ingested from that employer.
- **FR-013**: System MUST NOT propose or register a duplicate of an employer board already in the
  roster.
- **FR-014**: System MUST flag an employer whose board has produced no postings across a
  configurable number of consecutive runs as stale, without removing it automatically.

**Deduplication and merging**

- **FR-015**: System MUST recognise a posting read from an employer board as the same job as a
  previously ingested posting for the same opening from another source, and MUST keep one job
  record.
- **FR-016**: When merging, System MUST prefer the employer board's apply URL over an
  aggregator's, and MUST record every source the job was seen on.
- **FR-017**: When merging, System MUST preserve all user-created state on the existing job —
  application status, board position, scores, and generated documents.
- **FR-018**: System MUST NOT merge postings that are distinct openings, including identical
  titles at different employers and multiple genuinely separate reqs for the same role at one
  employer.

**Operations and failure handling**

- **FR-019**: System MUST isolate per-employer failures: one unreadable board MUST NOT abort a
  run or affect other employers' results.
- **FR-020**: System MUST distinguish and report, per employer, these outcomes: read
  successfully, board not found, board unreadable or shape changed, refused by host, and read but
  no open postings.
- **FR-021**: System MUST NOT report a run as successful when it read zero employers
  successfully.
- **FR-022**: System MUST expose each supported board vendor on the Sources screen with the same
  controls as existing sources: enable, disable, run on demand, health check, and last run
  status.
- **FR-023**: System MUST report, for each completed run, the number of employers read, postings
  found, postings new, and employers failed.
- **FR-024**: Users MUST be able to view the roster with each employer's vendor, last successful
  read, last posting count, and stale flag.

### Key Entities

- **Employer Board**: A registered employer's job board on one vendor. Holds the vendor, the
  employer's identifier on that vendor, the employer's display name, how it was added (proposed
  or pasted), when it was last read successfully, its recent posting counts, and its stale flag.
- **Board Candidate**: A proposed, not-yet-registered employer board inferred from an existing
  listing's apply URL. Holds vendor, employer identifier, the listing it was inferred from, and
  its state (proposed, accepted, rejected).
- **Board Vendor**: A supported job-board platform. Defines how one employer's postings are
  located and read, and how a URL is recognised as belonging to that vendor.
- **Board Run Result**: The outcome of one run over the roster. Holds per-employer outcomes and
  the aggregate counts reported to the operator.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a roster of registered employers, a run reads at least 95% of them successfully
  and reports a specific reason for every one it does not.
- **SC-002**: Zero runs require credentials, a stored session, a challenge-solving step, or any
  paid third-party service.
- **SC-003**: At least 100 job postings are discoverable through employer boards within the first
  week of use, from a roster built entirely from candidates the system proposed.
- **SC-004**: Roster candidates proposed from existing listings identify the correct employer and
  vendor at least 95% of the time, measured against the user's accept/reject decisions.
- **SC-005**: Registering an employer board takes the user under one minute from pasting a URL to
  seeing it in the roster.
- **SC-006**: When the same opening arrives from an aggregator and an employer board, the feed
  shows one job in at least 90% of cases, and shows two distinct jobs for genuinely distinct
  openings in 100% of cases.
- **SC-007**: No job's user-created state is lost to a merge.
- **SC-008**: At least 25% of the jobs the user reaches the application stage on originate from an
  employer board rather than an aggregator, within one month of use.
- **SC-009**: A single employer's unreadable board never reduces a run's other results, verified
  by a run in which at least one employer fails.
- **SC-010**: An operator can tell, from the Sources screen alone and without reading logs,
  whether a failing run failed because a board vanished, changed shape, refused the request, or
  simply had no openings.

## Assumptions

- Each board vendor appears as its own source on the Sources screen, matching the existing
  one-source-per-key model, so health, enablement, and run history stay per vendor. The employer
  roster is shared across vendors and stored once.
- Employer boards are read in full on every run — every currently open posting — rather than
  filtered by the user's search keywords. Filtering and ranking stay the job of the existing
  scoring pipeline, which already sees every ingested job. This is what makes the source family
  worth having: it surfaces openings a keyword search would have missed.
- Postings are public and published by employers for exactly this kind of consumption, so no
  robots exclusion, rate negotiation, or terms-of-service problem is expected beyond the
  system-wide pacing already enforced. Pacing is retained anyway.
- Merging reuses the deduplication the ingestion pipeline already applies across existing
  sources, extended rather than replaced. If today's matching is too weak to catch the
  aggregator↔board case, strengthening it is in scope for this feature.
- Roster candidate discovery runs against listings already stored, not by crawling the web for
  employers, so it costs nothing and touches no third-party host.
- The board vendors named in FR-002 publish machine-readable openings per employer without
  authentication. A vendor that turns out not to is dropped from launch scope and reported, not
  worked around with credentials or challenge-solving.
- This feature adds no auto-apply behavior of any kind. Apply URLs are surfaced for the user to
  act on manually, per the project's non-negotiable no-auto-apply principle.
- Feature 014 (browser-fidelity fetch ladder) is independent. This feature deliberately needs
  none of it, and neither feature blocks the other.
