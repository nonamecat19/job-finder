# Feature Specification: Manual Vacancy Add by URL

**Feature Branch**: `041-manual-vacancy-add`

**Created**: 2026-08-10

**Status**: Draft

**Input**: User description: "i need manual adding the vacancies in feed. for example i have djinni URL, you should scrape it and store like regular vacancy in feed (you can mark it as \"Manual\" in subscriptions)"

## Clarifications

### Session 2026-08-10

- Q: Which source does a manually added vacancy carry? → A: The real host source (e.g.
  `djinni`); attribution rides on a per-source "Manual" subscription, and unknown hosts on
  the fill-in path fall back to a `manual` source.
- Q: Is the add synchronous or queued? → A: Synchronous with a hard ~30s timeout; the
  operator waits and receives the vacancy, the failure reason, or the fill-in form directly.
  Per-host pacing still applies; a timeout stores nothing and permits retry.
- Q: Where does a manually added vacancy sort in the feed? → A: The extracted posted date is
  recorded truthfully and used by every age-sensitive feature; the feed additionally surfaces
  manual adds from the last 24 hours at the top regardless of their age.
- Q: What counts as "already in the feed" for a manual add? → A: The existing exact dedupe
  key only (company + title + resolved URL). A match is reported to the operator and opens
  the existing vacancy; near-matches are not merged and create a separate vacancy.
- Q: Does a manual add leave a run record? → A: Yes — every add, successful or failed, writes
  a run record in the same shape as an automated ingest run, so manual adds appear in run
  history and failures stay diagnosable after the operator dismisses the message.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Paste a posting URL and get it into the feed (Priority: P1)

The operator finds a vacancy outside the automated searches — a link from a Telegram channel,
a friend's referral, a posting on a board that has no subscription configured. They open the
feed, paste the posting URL into an "Add vacancy" field, and submit. The system reads the
posting page, extracts the same fields an automated run extracts (title, company, location,
remote flag, salary, description, posted date), and stores it as an ordinary job in the feed.
Within seconds the vacancy appears in the feed alongside automatically-collected ones, and
every downstream capability — matching, tailoring, tracker, enrichment — works on it with no
special-casing.

**Why this priority**: This is the whole feature. Everything else is refinement. Without it
the operator has no way to act on a vacancy the crawlers did not find, which is the single
largest hole in the current feed.

**Independent Test**: Paste a Djinni posting URL into the add field, submit, and confirm a
new feed card appears carrying the real title, company and description from that page, and
that opening its detail view offers the same actions as any other job.

**Acceptance Scenarios**:

1. **Given** an empty feed and a valid, reachable posting URL, **When** the operator submits
   it, **Then** exactly one new job appears in the feed with title, company and description
   read from the page, and the operator is shown the created vacancy.
2. **Given** a feed full of recent crawled vacancies and a posting three weeks old, **When**
   the operator adds that posting by hand, **Then** it appears at the top of the default feed
   without the operator scrolling or filtering, while still showing its real posting age.
3. **Given** a posting URL for a vacancy already in the feed (same company, title and URL),
   **When** the operator submits it, **Then** no duplicate is created, and the operator is
   told the vacancy already exists and is taken to the existing one.
4. **Given** a successfully added vacancy, **When** the operator opens it, **Then** matching,
   resume tailoring and tracker actions are available exactly as they are for an
   automatically-collected vacancy.
5. **Given** a URL that does not point at a readable job posting (404, a search page, a login
   wall), **When** the operator submits it, **Then** nothing is stored and the operator is
   shown a specific, human-readable reason for the failure.
6. **Given** a host that does not answer, **When** 30 seconds elapse, **Then** the operator is
   told the host timed out, nothing is stored, and resubmitting the same URL is permitted.

---

### User Story 2 - See manual additions attributed as "Manual" (Priority: P2)

Manually added vacancies keep the source of the host they came from, and are attributed to a
**Manual** subscription under that source, sitting alongside the configured crawling
subscriptions. The operator can tell at a glance which vacancies they added by hand and which
the crawlers found. Manual subscriptions are visible in the subscriptions list, are not
schedulable, and are never run by the scheduler.

**Why this priority**: Attribution matters for judging where results come from, but the
vacancy is already useful without it. It ships right after P1.

**Independent Test**: Add two vacancies from the same host by URL, open the
subscriptions/sources view, and confirm a Manual subscription exists under that source
showing those two vacancies as its origin, with no cron schedule and no "run now" control.

**Acceptance Scenarios**:

1. **Given** at least one manually added vacancy, **When** the operator opens the
   subscriptions view, **Then** a "Manual" subscription is listed under that vacancy's source
   with the count and time of the most recent manual addition.
2. **Given** a Manual subscription, **When** the scheduler ticks, **Then** it is never
   executed and its presence changes no automated run.
3. **Given** a Manual subscription, **When** the operator views it, **Then** it cannot be
   given a cron schedule, cannot be edited into a crawling subscription, and cannot be
   deleted or disabled while vacancies are attributed to it.
4. **Given** manual adds from two different hosts, **When** the operator opens the
   subscriptions view, **Then** each host's source shows its own Manual subscription, and no
   host's manual count includes the other's.
5. **Given** manually added vacancies from several hosts, **When** the operator filters the
   feed to "Manual", **Then** all of them are shown and no crawled vacancy is.

---

### User Story 3 - Recover when the page cannot be read (Priority: P3)

Some posting pages sit on a host the system has no reader for, or are behind a login, a bot
challenge, or a layout the extractor does not understand. When extraction is impossible,
fails, or returns obviously incomplete data (no title, or no description), the operator is
not left stuck: they are offered a form pre-filled with
whatever *was* extracted and can complete or correct the missing fields by hand, then save.
The resulting vacancy is stored the same way as a fully-extracted one.

**Why this priority**: A useful escape hatch, but only after the happy path proves itself.
Until it ships, an unreadable or unknown host is a clear rejection message (FR-018) — a
survivable outcome, since the operator can still act on the posting outside the system.

**Independent Test**: Submit a URL whose page yields no usable description, confirm the
fill-in form appears carrying the partial data, complete the required fields, save, and
confirm the vacancy lands in the feed.

**Acceptance Scenarios**:

1. **Given** a URL whose page cannot be read at all, **When** the operator submits it, **Then**
   they are offered the manual fill-in form with the URL retained.
2. **Given** the fill-in form, **When** the operator saves without a title, company or
   description, **Then** the save is rejected with a message naming the missing fields.
3. **Given** a completed fill-in form, **When** the operator saves, **Then** the vacancy is
   stored, appears in the feed, and is attributed to Manual exactly like an extracted one.

---

### Edge Cases

- **URL that is a search page, not a posting** (e.g. a Djinni preset-search URL, an Indeed
  results page). Rejected with a message that says it looks like a search, and points the
  operator at creating a subscription instead.
- **Posting already in the feed.** No duplicate; the operator is taken to the existing
  vacancy. This holds whether the existing one came from a crawl or an earlier manual add.
- **A near-match already in the feed** — the same role re-posted under a slightly different
  title, or the same job on a second board. A separate vacancy is created. Accepted cost of
  never silently swallowing a deliberate add; the operator can delete the one they don't want.
- **A manual add that a crawl later finds under a subscription.** The crawl's own dedupe
  recognises it and records a repost against the existing vacancy rather than duplicating it;
  the vacancy keeps its Manual attribution.
- **Posting that was previously in the feed and is now hidden or archived.** Treated as
  already existing; the operator is told its current state rather than getting a fresh copy.
- **URL with tracking parameters or a redirect chain** (shortener, `utm_*`, an aggregator
  bounce link). The final destination is what gets read and recorded, so the same posting
  reached via two different links deduplicates to one vacancy.
- **Very long or truncated description**, or a page that renders its description only after
  scripting. The extractor uses the same retrieval path the crawlers use, so it succeeds
  wherever they succeed and reports a specific reason where they do not.
- **Non-HTTP or malformed input** (`ftp://`, plain text, an empty string). Rejected before
  any network request.
- **URL on a host with no reader** (a company careers page, a niche board, a Telegram-shared
  link). Not a hard rejection — the operator is told there is no reader and lands on the
  fill-in form with the URL kept.
- **A host reader exists but only knows how to read search pages, not single postings.**
  Treated the same as no reader: fill-in form, with a reason saying so.
- **The same URL submitted twice in quick succession** (double-click, retry). Only one
  vacancy results.
- **A slow host, or one whose pacing queue is busy behind a running crawl.** The operator
  waits at most 30 seconds and is then told the host did not answer in time; nothing is
  stored and the URL can be resubmitted. The manual add never jumps the pacing queue.
- **The operator navigates away or cancels mid-add.** No partial vacancy is stored; the
  submission is either fully applied or not at all.
- **Extraction succeeds but the page is not a job posting at all** (a company homepage, a
  blog post). Missing required fields drive it into the fill-in path rather than storing a
  junk vacancy.
- **The operator deletes a manually added vacancy.** It disappears from the feed like any
  other; re-adding the same URL afterwards is allowed and creates it again.

## Requirements *(mandatory)*

### Functional Requirements

**Adding**

- **FR-001**: Operators MUST be able to add a vacancy to the feed by submitting a single
  posting URL, from the feed view, without configuring a subscription.
- **FR-002**: The system MUST validate the submitted URL before any network request: it must
  parse, and use `http` or `https`. Anything else is rejected immediately with a reason.
- **FR-003**: The system MUST read the posting page and extract the same fields an automated
  run extracts: title, company, location, remote flag, raw salary, description, posted date,
  and the canonical posting URL.
- **FR-003a**: Reading the page MUST be synchronous with respect to the operator's
  submission: the operator waits and is given the created vacancy, a failure reason (FR-018),
  or the fill-in form (FR-019) as the direct outcome of submitting. Manual add MUST NOT
  return a bare "processing" acknowledgement.
- **FR-003b**: A manual add MUST be bounded by a hard timeout of 30 seconds, covering
  retrieval and extraction including any time spent waiting on per-host pacing. On timeout,
  nothing is stored, the operator is told the host did not answer in time, and the same URL
  may be submitted again immediately.
- **FR-003c**: Manual add MUST respect the same per-host pacing and retrieval escalation as
  automated runs — it MUST NOT bypass throttling to make an interactive request faster.
- **FR-003d**: The post-ingest processing of FR-010 MUST NOT be bounded by FR-003b. Once the
  vacancy is stored the operator is done waiting; matching and enrichment complete on their
  own schedule, exactly as for a crawled vacancy.
- **FR-004**: Where the URL's host belongs to a source the system already knows how to read,
  the system MUST use that source's existing extraction behaviour, so a manually added
  vacancy from that host is indistinguishable in content from a crawled one.
- **FR-005**: The system MUST reject a URL that is recognisably a search or listing page
  rather than a single posting, with a message that says so and points the operator at
  subscriptions.
- **FR-006**: The system MUST follow redirects and strip tracking-only parameters before
  recording the posting URL, so the same posting submitted via two different links resolves
  to one vacancy.
- **FR-007**: The system MUST decide "already in the feed" using the existing exact dedupe
  key — company, title and the resolved posting URL — and nothing else. A submission matching
  an existing vacancy MUST NOT create a second one; the operator is told it already exists
  and is given the existing vacancy.
- **FR-007a**: Manual add MUST NOT apply the crawlers' near-match merging. A submission that
  resembles but does not exactly match an existing vacancy creates a separate vacancy, so no
  manual add is ever silently absorbed into a different job.
- **FR-007b**: A duplicate MUST be reported as a distinct, non-error outcome — "this is
  already in your feed", not a failure — and MUST take the operator to the existing vacancy.
  Deduplication MUST NOT be silent.
- **FR-008**: Concurrent or repeated submissions of the same URL MUST result in at most one
  vacancy.

**Storage and downstream behaviour**

- **FR-009**: A manually added vacancy MUST be stored as an ordinary vacancy, carrying no
  field or state that downstream capabilities must special-case.
- **FR-010**: A manually added vacancy MUST enter the same post-ingest processing as a
  crawled one — matching, scoring, enrichment and any other automatic step that runs on new
  vacancies — with no additional operator action.
- **FR-011**: Matching, resume tailoring, the tracker, and every other per-vacancy action
  MUST work on a manually added vacancy exactly as on a crawled one.
- **FR-012**: A manually added vacancy MUST carry the source of the host it actually came
  from — a Djinni posting added by hand is a Djinni vacancy — so it is indistinguishable from
  a crawled vacancy of the same host for dedupe, filtering and per-source behaviour.
- **FR-012a**: A vacancy saved through the fill-in path for a host with no reader MUST carry
  a dedicated `manual` source, since no real host source applies.
- **FR-012b**: Manual attribution MUST ride on the **subscription**, not the source: each
  manually added vacancy is attributed to a Manual subscription belonging to its source. The
  system MUST record when each vacancy was added by hand.

**The Manual subscription**

- **FR-013**: The system MUST present the Manual subscriptions in the subscriptions view,
  each summarising how many vacancies were added by hand under that source and when the most
  recent was added.
- **FR-014**: A Manual subscription MUST NOT be schedulable and MUST NOT be executed by the
  scheduler, by a "run now" action, or by any health check.
- **FR-015**: A Manual subscription MUST come into existence implicitly on first manual add
  for its source — the operator never creates one — and MUST NOT be editable into a crawling
  subscription (no URL, no cron).
- **FR-016**: A Manual subscription MUST NOT be deletable or disablable while vacancies are
  attributed to it, and no operation on it may delete or hide those vacancies.
- **FR-017**: The feed MUST allow filtering to show only manually added vacancies, across all
  sources, using the Manual attribution rather than the source key.

**Dates and feed placement**

- **FR-017a**: The posted date MUST be recorded as extracted from the posting, never
  substituted with the time of the manual add. Where the page exposes no posted date, the
  vacancy MUST have no posted date, exactly as for a crawled vacancy from the same host.
- **FR-017b**: Every age-sensitive capability — post-age signals, ghost-job scoring, and any
  other feature reading the posting's age — MUST see the truthful posted date, or its
  absence, with no manual-add special case.
- **FR-017c**: The feed MUST surface vacancies added by hand within the last 24 hours at the
  top of the default feed ordering, regardless of their posted date, so an operator always
  sees the vacancy they just added without searching for it.
- **FR-017d**: This surfacing MUST expire on its own: past 24 hours from the add, a manually
  added vacancy takes its natural place in the feed ordering by posted date. It MUST NOT
  apply to any non-default ordering the operator explicitly chooses.

**Observability**

- **FR-017e**: Every manual add MUST write a run record in the same shape as an automated
  ingest run — when it started and finished, how many postings were found and how many were
  new, and the failure reason where one applies — attributed to the Manual subscription.
- **FR-017f**: A failed or duplicate add MUST write a run record too, carrying its outcome,
  so a host that intermittently blocks or times out is diagnosable after the operator has
  dismissed the on-screen message.
- **FR-017g**: Manual runs MUST appear in run history and any run-listing view alongside
  automated runs, distinguishable by their Manual subscription, and MUST NOT distort
  per-source health: a failed manual add MUST NOT count toward the consecutive-failure
  threshold that flags a source unhealthy.
- **FR-017h**: The count and "most recent addition" of FR-013 MUST derive from these run
  records, so the subscriptions view reads manual and crawling subscriptions the same way.
- **FR-017i**: A manually added vacancy MUST carry the same per-job activity trail a crawled
  vacancy carries, with no manual-add gap in it.

**Failure and recovery**

- **FR-018**: When the page cannot be retrieved or cannot be interpreted, the system MUST
  report a specific, human-readable reason distinguishing at least: unreachable, timed out,
  blocked or login-required, not a job posting, no reader for this host, and
  readable-but-incomplete.
- **FR-019**: When extraction yields no title, no company, or no description — or when no
  reader exists for the host — the system MUST offer the operator a fill-in form
  pre-populated with whatever was extracted, retaining the submitted URL.
- **FR-020**: Saving a hand-filled vacancy MUST require title, company and description, and
  MUST reject a save that is missing any of them, naming the missing fields.
- **FR-021**: A failed add MUST leave no vacancy, no partial record, and no state that blocks
  re-submitting the same URL.

**Scope of accepted hosts**

- **FR-022**: Automatic extraction MUST be attempted for every host the system already knows
  how to read — every configured job source, including employer board vendors. No new
  per-host reader is introduced by this feature.
- **FR-023**: A URL on a host the system has no reader for MUST NOT be rejected outright. The
  operator is told no reader exists for that host and is offered the fill-in form (FR-019)
  with the URL retained, so any posting anywhere can still reach the feed.
- **FR-024**: Generic, host-agnostic extraction for unknown hosts is explicitly out of scope.
  Unknown hosts go to the fill-in form, not to a best-effort parse.

### Key Entities

- **Vacancy**: An ordinary job in the feed, carrying the source of the host it came from.
  Manually added vacancies are the same entity, distinguished only by the Manual subscription
  they are attributed to and the timestamp of the addition.
- **Manual subscription**: A non-crawling, non-schedulable subscription belonging to a
  source, existing so manual additions are attributable and filterable. Holds no URL to crawl
  and no schedule. Created implicitly on first manual add for that source. At most one per
  source.
- **`manual` source**: A source that stands in for hosts the system has no reader for, so
  fill-in vacancies have somewhere to belong. Never crawled, never health-checked.
- **Manual add attempt**: A submission — the URL, its outcome (created, duplicate, needs
  fill-in, failed), and the failure reason where one applies — recorded durably as a run
  against the Manual subscription, in the same shape as an automated ingest run.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can go from holding a posting URL to seeing that vacancy in the
  feed in under 30 seconds, in no more than 3 interactions (paste, submit, view). A submit
  never leaves the operator waiting longer than 30 seconds for *some* definite outcome —
  vacancy, reason, form, or timeout.
- **SC-002**: For a posting on a host the system already reads, at least 95% of manual adds
  produce a vacancy whose title, company and description match what an automated run would
  have produced for the same posting. Hosts with no reader are excluded from this bar — they
  go to the fill-in path by design.
- **SC-003**: Submitting the same posting twice — by identical URL, by a tracked variant, or
  concurrently — never produces more than one vacancy, in 100% of attempts, and the operator
  is told it already exists rather than the add appearing to have done nothing.
- **SC-003a**: No manual add is ever merged into a vacancy the operator did not submit: 0%
  silent absorption of a manual add into a near-matching existing job.
- **SC-004**: 100% of failed adds show a reason naming which of the six failure kinds
  occurred (FR-018), and leave no vacancy behind.
- **SC-005**: 100% of manually added vacancies support matching, tailoring and tracker
  actions on first attempt, with no operator step beyond the add itself.
- **SC-006**: Manual additions are attributable: an operator can list every manually added
  vacancy, and only those, in one interaction from the feed.
- **SC-006a**: A freshly added vacancy is visible in the default feed without scrolling or
  filtering, in 100% of adds, whatever its posting age.
- **SC-006b**: Age-sensitive features report the same age for a manually added vacancy as
  they would for the identical posting collected by a crawl, in 100% of cases.
- **SC-007**: Adding a vacancy manually never triggers a crawl and never changes the timing
  or results of any scheduled run, and no number of failed manual adds ever flags a source
  unhealthy.
- **SC-007a**: 100% of manual adds — created, duplicate, or failed — are reconstructable from
  run history afterwards, without the operator having kept the on-screen message.
- **SC-008**: When extraction is incomplete, an operator can still complete the add by hand
  in under 2 minutes without re-finding the posting URL.

## Assumptions

- **Single operator, no permissions model.** The system is single-tenant as it stands; manual
  add is available to whoever is using the dashboard, with no new role or permission.
- **Manual subscriptions are implicit.** One comes into existence per source on first use
  rather than being something the operator sets up, so the first add is not gated behind
  configuration.
- **Existing duplicate rules are reused unchanged.** Manual add does not introduce its own
  notion of "same vacancy"; it inherits whatever the ingest path already uses.
- **Existing retrieval behaviour is reused unchanged.** Manual add fetches pages the same way
  automated runs do, including the same politeness and escalation behaviour, so it succeeds
  and fails on the same hosts.
- **Manual adds are low-volume.** Expected on the order of a handful per day, so no bulk
  paste, no CSV import, and no batching are in scope for this feature.
- **Adding one URL at a time.** Multi-URL paste is out of scope for v1.
- **Login-walled postings are out of scope.** Where the crawlers cannot read a host without
  credentials, manual add will not either; the fill-in path is the answer, not per-host
  credentials.
- **No new per-host readers.** Manual add reaches exactly the hosts the system already
  reaches. Extending that set is a separate feature, and every unknown host degrades to the
  fill-in form rather than failing.
- **Some existing readers may only handle search pages.** Where a source has no
  single-posting read path today, that host degrades to fill-in until one is added; the
  feature does not block on retrofitting every source.
- **No new notification behaviour.** A manual add does not generate a "fresh match"
  notification, because the operator already knows about the vacancy.
- **Manually added vacancies age and are pruned like any other**, under whatever retention
  the feed already applies.
