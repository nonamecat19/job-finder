# Feature Specification: work.ua and robota.ua Job Source Adapters

**Feature Branch**: `001-workua-robotaua-adapters`

**Created**: 2026-07-16

**Status**: Draft

**Input**: User description: "i need to create work.ua and rabota.ua adapters similar to djinni/dou"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover work.ua jobs from a keyword search (Priority: P1)

A job seeker targeting the Ukrainian market opens the Sources page, enables work.ua as a job source, and runs a saved search with keywords (and optionally a remote-only filter). Listings from work.ua appear in their job list alongside results from the sources they already use, deduplicated and scored the same way.

**Why this priority**: work.ua is the largest general-purpose Ukrainian job board and its listings are publicly readable without an account, so it delivers the most new coverage for the least setup friction. On its own it is a complete, shippable increment.

**Independent Test**: Enable only work.ua, run a keyword search, and confirm jobs appear in the job list with title, company, location, and a working link back to the original posting. No other source needs to be enabled.

**Acceptance Scenarios**:

1. **Given** work.ua is enabled and a saved search with keywords exists, **When** the search runs, **Then** matching work.ua listings appear in the job list with title, company, source link, and (when the listing shows one) location and salary.
2. **Given** a saved search with the remote-only filter on, **When** the search runs against work.ua, **Then** only listings the board marks as remote are returned.
3. **Given** the same work.ua listing is returned by two consecutive search runs, **When** results are ingested, **Then** it appears once in the job list, not duplicated.
4. **Given** work.ua returns a page with no recognizable listings, **When** the search runs, **Then** the run completes without error, records zero results, and logs a warning that the page layout may have changed.

---

### User Story 2 - Discover robota.ua jobs from a keyword search (Priority: P2) — DEFERRED

> **Status (2026-07-16): deferred, not implemented.** Live probing during planning found robota.ua entirely behind a Cloudflare managed bot challenge — every path returns a JS challenge, including `robots.txt` itself. No plain-HTTP path exists, and reaching the data would mean circumventing the operator's bot protection. Parked pending official/partner API access. Evidence and rejected alternatives in [research.md](./research.md). The story is retained below unchanged so it can be picked up if that access is granted.

The same job seeker enables robota.ua as a second Ukrainian source and runs their saved searches against it, widening coverage to postings that appear on robota.ua but not work.ua.

**Why this priority**: Same user value as Story 1 but on a second board; it is additive coverage rather than the first foothold in the market, and the two boards' listings overlap substantially. Ships independently of Story 1.

**Independent Test**: Enable only robota.ua, run a keyword search, and confirm jobs appear in the job list with title, company, and a working link back to the original posting.

**Acceptance Scenarios**:

1. **Given** robota.ua is enabled and a saved search with keywords exists, **When** the search runs, **Then** matching robota.ua listings appear in the job list with title, company, source link, and (when available) location and salary.
2. **Given** a saved search with the remote-only filter on, **When** the search runs against robota.ua, **Then** only listings the board marks as remote are returned.
3. **Given** robota.ua is unreachable or rejects the request, **When** the search runs, **Then** the run is recorded as failed for that source only, and results from other enabled sources are still ingested.

---

### User Story 3 - Full job descriptions filled in after discovery (Priority: P3)

Search results from a listing page carry only a short teaser. Shortly after a work.ua or robota.ua job is discovered, the user opens it and sees the complete description, salary, location, and posting date pulled from the original posting — enough to decide whether to apply and enough for the scoring and document-generation flows to work well.

**Why this priority**: Discovery (Stories 1-2) is usable without it, but shallow descriptions weaken match scoring and tailored document generation, which are the product's core value. Matches the existing behavior for djinni and dou.

**Independent Test**: Discover a job from either new source, wait for background enrichment, and confirm the job's description grows from teaser to full text and that salary/location/posted date are populated when the posting shows them.

**Acceptance Scenarios**:

1. **Given** a newly discovered work.ua or robota.ua job with a teaser description, **When** background enrichment runs, **Then** the job's description is replaced with the full posting text and the posting date is populated when the posting shows one.
2. **Given** a job whose original posting has been removed, **When** enrichment runs, **Then** the job keeps its teaser description, the failure is logged, and no other job's enrichment is blocked.
3. **Given** many jobs from one source are queued for enrichment, **When** enrichment runs, **Then** requests to that board are paced so the board is not hit faster than a human browsing it.

---

### User Story 4 - Reuse a saved filter from the board itself (Priority: P4)

A user who has already tuned a filter on work.ua or robota.ua pastes that filter's URL into a subscription instead of re-describing the filter as keywords, and the system ingests everything that filter returns, following pagination.

**Why this priority**: Convenience for power users and parity with djinni/dou subscriptions, but the keyword path already covers the primary need.

**Independent Test**: Paste a board filter URL into a subscription, run it, and confirm listings from every page of that filter appear in the job list.

**Acceptance Scenarios**:

1. **Given** a subscription holding a work.ua or robota.ua filter URL, **When** it runs, **Then** listings from that filter are ingested across multiple pages.
2. **Given** a filter URL whose pagination loops back to the first page, **When** the subscription runs, **Then** pagination stops instead of looping indefinitely.
3. **Given** a malformed or non-listing URL, **When** the subscription runs, **Then** the run fails with a message naming the bad URL rather than ingesting garbage.

---

### Edge Cases

- A listing shows no company name → the job is still ingested with a placeholder company rather than dropped.
- A listing shows no salary or no location → those fields stay empty; the job is still ingested.
- Listing text is Ukrainian or Russian Cyrillic → titles, descriptions, and stored raw content preserve every character intact, with no mangled or truncated characters.
- A board changes its page layout so nothing parses → the run completes with zero results and a warning that layout may have changed, rather than an error that hides the cause.
- A board rate-limits or blocks the requests → the run fails for that source alone; other sources are unaffected and the failure is visible on the Sources page.
- The same posting appears on both boards → each is ingested under its own source; cross-board deduplication is out of scope.
- A search returns thousands of listings → pagination stops at a bounded page cap so a single run cannot grow without limit.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST offer work.ua and robota.ua as separately enableable job sources, each independently enabled, disabled, and configured without affecting the other or any existing source.
- **FR-002**: System MUST return job listings from each board for a keyword search, populating title, company, source link, and — when the board shows them — location, salary text, and a remote indicator.
- **FR-003**: System MUST honor the remote-only search filter for both boards, returning only listings the board identifies as remote.
- **FR-004**: System MUST give each ingested job a stable identifier derived from the original posting, so that re-running the same search does not duplicate jobs already in the list.
- **FR-005**: System MUST preserve non-Latin (Cyrillic) text intact through parsing and storage, including any truncated raw content.
- **FR-006**: System MUST treat a missing optional field (salary, location, company, posting date) as absent and still ingest the job, rather than discarding it or failing the run.
- **FR-007**: System MUST complete a run that yields zero recognizable listings without erroring, and MUST emit a warning distinguishing "nothing matched" from "the page could not be understood".
- **FR-008**: System MUST confine a failure of one board (unreachable, blocked, layout change) to that board's run, leaving other enabled sources' results unaffected.
- **FR-009**: System MUST report each board's reachability on the Sources page so the user can tell a broken source from an empty search.
- **FR-010**: System MUST fill in a discovered job's full description, and when the posting shows them its salary, location, and posting date, by revisiting the original posting after discovery.
- **FR-011**: System MUST pace repeated requests to a single board at a configurable interval, defaulting to a rate no faster than a human browsing the site.
- **FR-012**: System MUST accept a board-native filter URL as a subscription source and ingest its listings across pages, stopping at a bounded page cap or when a page repeats content already seen.
- **FR-013**: System MUST reject a malformed subscription URL with a message naming the offending URL.
- **FR-014**: System MUST keep any per-source credential or session value the user supplies confidential — never displayed back in full, never written to logs.
- **FR-015**: System MUST NOT submit applications, send messages, or take any action on a listing on either board; both adapters are read-only discovery.
- **FR-016**: System MUST make each board's source configuration settable without a redeploy, via the same per-source configuration surface the existing sources use.

### Key Entities

- **Job Source**: A board the system can search — here, work.ua and robota.ua. Has a stable key, a kind (discovery by page-reading vs. published feed), an enabled flag, per-source configuration, and a health state.
- **Normalized Job**: A listing after conversion to the shared shape all sources produce: source key, external identifier, title, company, location, remote flag, salary text, link to the original posting, description, and retained raw source content.
- **Search Query**: What a run asks a source for: keywords, remote-only flag, and optionally a board-native filter URL that replaces the keyword search.
- **Subscription**: A saved board-native filter URL that runs on a schedule and ingests everything the filter returns.
- **Job Detail Patch**: The fuller set of fields recovered from an individual posting after discovery — full description, salary, location, remote flag, posting date — applied over the shallow listing.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can enable either board and see its listings in their job list within 5 minutes of first opening the Sources page, with no configuration beyond enabling the source.
- **SC-002**: For a keyword search a human can reproduce on the board itself, at least 90% of the listings on the board's first result page appear in the user's job list.
- **SC-003**: Every ingested job carries a title, a company (or an explicit placeholder), and a link that opens the original posting — 100% of rows, no blanks.
- **SC-004**: Re-running the same search twice in a row adds zero duplicate jobs.
- **SC-005**: Once a job's enrichment begins, its full description is stored within 30 seconds. For a keyword search returning a typical single page of results (~15 jobs), every discovered job carries a full description rather than a teaser within 10 minutes of discovery.

  > **Revised 2026-07-16.** Previously read "within 10 minutes of discovery, at least 95% of jobs… carry a full description". That was unachievable and contradicted FR-011: the board's mandated 2-second pacing serialises enrichment at ~30 jobs/minute, capping a 10-minute window at ~300 jobs, while a large subscription ingest can discover ~700. The criterion now scopes the 10-minute promise to a keyword-search-sized batch and states the per-job latency separately. Large ingests are rate-bound by the board's crawl-delay by design and take proportionally longer (roughly 35 minutes per 1000 jobs); that is correct behaviour, not a failure.
- **SC-006**: When a board changes its layout so nothing parses, the resulting run is diagnosable from its logged warning alone, without reproducing the failure by hand.
- **SC-007**: A run against one board failing leaves 100% of other enabled sources' results intact for that same search.
- **SC-008**: Neither board blocks or rate-limits the system during a normal day's scheduled runs.
- **SC-009**: Adding these two sources requires no change to how existing sources are configured, enabled, or displayed — zero regressions in existing source behavior.

## Assumptions

- **"rabota.ua" means robota.ua**: The site the user named now operates as robota.ua (the older rabota.ua domain redirects). The adapter targets the current site; the user-visible source name follows the current branding.
- **Parity with djinni/dou is the target**: The user's "similar to djinni/dou" is read as parity across all four capabilities those adapters have today — keyword search, remote filter, health check, board-native subscription URLs with paged ingestion, and post-discovery detail enrichment — not merely a listing search.
- **Both boards are general-purpose, not dev-only**: Unlike djinni/dou, these boards carry listings across all industries. No implicit filtering to tech roles is applied; keyword filters remain the user's responsibility.
- **Listings are publicly readable**: ~~Both boards show listings without an account.~~ **Falsified for robota.ua on 2026-07-16** — it is gated by a Cloudflare managed challenge and is not readable by any plain-HTTP client (see [research.md](./research.md)). Holds for work.ua, which serves listings over plain HTTP with no account and publishes a `Crawl-delay: 2` in its `robots.txt`. Credential support remains unnecessary for work.ua.
- **Read-only discovery**: Per the project's non-negotiable no-auto-apply principle, these adapters only read listings. Applying stays a human action outside this feature.
- **Ukrainian/Russian listing content is the norm**: Non-Latin text handling is a baseline requirement, not an edge case.
- **Existing plumbing is reused**: Job list, deduplication, scoring, enrichment scheduling, and the Sources page already exist and work for djinni/dou; this feature adds two sources to them and does not redesign them.
- **Cross-board deduplication is out of scope**: The same posting cross-listed on both boards is expected to appear twice, once per source, as it would for any two existing sources.
- **Per-source pacing is configurable**: Following the existing djinni detail-delay precedent, each board's request interval is tunable without a code change.
- **Boards may change their layout at any time**: Parsing is best-effort and defensive by design; a layout change degrades a source to zero results with a warning, and is expected maintenance rather than a defect in this feature.

## Dependencies

- The existing job-source extensibility point, source registry, and Sources configuration UI.
- The existing page-fetching service used by the djinni and dou adapters (shared pacing, headers, error handling).
- The existing background enrichment flow that applies detail patches to discovered jobs.
- The existing subscription mechanism that runs board-native filter URLs on a schedule.
- Both boards remaining publicly reachable and permitting automated reading at the pacing above — an external dependency outside the project's control.
