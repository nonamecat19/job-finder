# Feature Specification: Browser-Fidelity Retrieval and Escalation Ladder

**Feature Branch**: `014-browser-fidelity-fetch-ladder`

**Created**: 2026-07-25

**Status**: Draft

**Input**: User description: "Make scraped sources retrievable without paid services by having
requests behave like a real browser, escalating to heavier retrieval only when challenged, and
reporting blocks instead of failing silently."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Blocked sources start returning listings (Priority: P1)

Several sources are refused or handed a challenge page instead of content, so the user sees no
listings from them at all. The cause is not the pace of the requests — it is that the requests do
not resemble a real person's browser: they announce a stale browser version, carry a fraction of
the headers a browser sends, and arrive as a brand-new visitor with no history every single time.
Presenting requests the way a real browser does, and remembering a host between visits the way a
browser does, should get those sources answering.

**Why this priority**: Sources that return nothing are the largest hole in the feed today. Every
other story here is about keeping that fix healthy over time.

**Independent Test**: Point a currently-refused source at its normal target, run it, and confirm
real listings come back rather than a refusal or a challenge page — repeatedly, across several
runs on different days.

**Acceptance Scenarios**:

1. **Given** a source that is currently refused on every attempt, **When** it runs with
   browser-fidelity retrieval, **Then** it returns real listings rather than a refusal or
   challenge page.
2. **Given** a host that issued a visitor credential on a previous run, **When** a later run
   contacts the same host, **Then** the run reuses that credential rather than arriving as a new
   visitor.
3. **Given** a stored visitor credential for a host, **When** it is reused, **Then** it is
   presented with exactly the same browser identity it was issued to, and never with a different
   one.
4. **Given** any retrieval attempt, **When** the request is made, **Then** its declared browser
   identity is internally consistent — version, platform, and capability claims all agree, and
   all describe a browser release that is current.
5. **Given** repeated runs against one host, **When** their timing is compared, **Then** requests
   are not evenly spaced.
6. **Given** the system is upgraded to a newer browser identity, **When** the change takes effect,
   **Then** visitor credentials issued to the old identity are discarded rather than reused under
   the new one.

---

### User Story 2 - Escalate only when actually challenged (Priority: P1)

Most hosts answer a well-behaved browser-like request directly. A few will not, and only those
few justify the cost of driving a real browser or handing the page to the challenge-solving
service that is already deployed alongside the system but currently connected to nothing. The
system should try the cheap way first, escalate only on a genuine challenge, and remember which
rung a given host actually needs so it stops paying for discovery on every run.

**Why this priority**: Without escalation, the hardest sources stay dark. Without escalation
being *conditional and remembered*, every run pays browser-startup cost for hosts that never
needed it, and the heavier rungs get used often enough to look abnormal themselves.

**Independent Test**: Run against a host that challenges the cheap request and confirm the run
escalates, succeeds, records which rung worked, and then goes straight to that rung on the next
run — while a host that answers the cheap request never escalates at all.

**Acceptance Scenarios**:

1. **Given** a host that answers a browser-fidelity request directly, **When** a run contacts it,
   **Then** no escalation occurs.
2. **Given** a host that returns a challenge, **When** the run detects it, **Then** the run
   escalates to the next retrieval method and retries the same page.
3. **Given** an escalation that succeeds, **When** a later run contacts the same host, **Then** it
   begins at the rung that last worked instead of rediscovering it.
4. **Given** a host recorded as needing a heavy rung, **When** enough time passes or its recorded
   preference is cleared, **Then** the system re-tests the cheaper rung so a host is not
   permanently stuck on an expensive method.
5. **Given** every rung has been tried and all were challenged, **When** the run gives up,
   **Then** it reports the page as blocked and does not keep retrying within that run.
6. **Given** a heavier rung is unavailable — its service is not running or not configured —
   **When** a run would escalate to it, **Then** the run reports the page as blocked with that
   reason and does not fail the whole run.
7. **Given** a source that authenticates with the user's real account credentials, **When** it is
   refused or challenged, **Then** the system MUST NOT escalate and MUST report the block for the
   user to resolve manually.
8. **Given** a run whose escalation drives a real browser, **When** it renders a third-party page,
   **Then** that rendering is isolated from the rendering path used for the user's own documents.

---

### User Story 3 - A blocked source never looks like an empty one (Priority: P1)

The most expensive failure is the quiet one: a source that used to return listings starts
returning a challenge page, the parser finds no listings in it, and the run reports success with
zero results. The user concludes there are no jobs. A source that cannot be read must say so.

**Why this priority**: Equal to the others because it is what makes the whole feature
maintainable. A silent failure means the user stops trusting the feed and cannot tell which
source to fix.

**Independent Test**: Feed a source a challenge page and a refusal in place of real content, and
confirm each run is reported as blocked with its reason — never as a successful empty run.

**Acceptance Scenarios**:

1. **Given** a source whose every page was blocked, **When** the run finishes, **Then** it is
   reported as blocked with a reason, not as a successful run with zero listings.
2. **Given** a source that returned listings on recent runs, **When** a run returns zero listings
   with no block detected, **Then** the source is flagged as needing attention rather than
   silently accepted.
3. **Given** a run in which some pages were blocked and others were read, **When** it finishes,
   **Then** it reports partial success with the count of blocked pages, and keeps the listings it
   did read.
4. **Given** a host that has blocked the system on several consecutive runs, **When** the next run
   is due, **Then** the host is left alone for a cooling-off period instead of being contacted
   again immediately.
5. **Given** a host in a cooling-off period, **When** the operator triggers a run on demand,
   **Then** they are told the host is cooling off and for how long, and may override it
   deliberately.
6. **Given** a host that asks the system to wait a stated amount of time, **When** the run reads
   that instruction, **Then** the system waits at least that long before contacting the host
   again.

---

### User Story 4 - The operator can see how each source is being retrieved (Priority: P2)

When a source misbehaves, the operator needs to know which retrieval method it is using, when it
was last blocked and why, and whether it is cooling off — without reading logs.

**Why this priority**: This makes the feature diagnosable, but the feature still delivers its
value without the surface.

**Independent Test**: Block a source, then read the Sources screen and confirm it shows the
retrieval method, the last block reason and time, and any cooling-off period.

**Acceptance Scenarios**:

1. **Given** the Sources screen, **When** the operator views a scraped source, **Then** they see
   which retrieval method it is currently using per host.
2. **Given** a source that was blocked, **When** the operator views it, **Then** they see when and
   why, and whether a cooling-off period is in effect.
3. **Given** a source stuck on a heavy retrieval method, **When** the operator clears its recorded
   preference, **Then** the next run re-tests the cheap method.
4. **Given** stored visitor credentials for a host, **When** the operator clears them, **Then** the
   next run starts a fresh visit to that host.

---

### User Story 5 - Volume stays low enough to be unremarkable (Priority: P2)

Speed is explicitly not wanted. The system's best protection is that it looks like one person
casually browsing from one home connection, and its worst risk is that a growing roster of
sources quietly multiplies into a volume no person would produce.

**Why this priority**: It protects the P1 gains from being undone as more sources are added, but
today's volume is already low enough that this is a hardening measure rather than a fix.

**Independent Test**: Run every enabled source concurrently and confirm no single host receives
more than its configured daily request budget, and that pacing per host holds regardless of how
many sources target it.

**Acceptance Scenarios**:

1. **Given** several sources targeting one host, **When** they run at once, **Then** the host's
   pacing is enforced across all of them together, not per source.
2. **Given** a host with a daily request budget, **When** the budget is exhausted, **Then**
   further requests to that host are deferred to the next period and reported as deferred, not as
   failures.
3. **Given** a host that publishes a crawl delay, **When** the system contacts it, **Then** it
   honors the published delay if that is slower than its own pacing.
4. **Given** any retrieval, **When** the interval since the previous request to that host is
   measured, **Then** it varies rather than repeating a fixed value.

---

### Edge Cases

- A stored visitor credential expires, is invalidated, or is bound to a network address that has
  changed: the run detects the failure, discards the credential, and starts a fresh visit rather
  than looping on a dead one.
- A host answers with real content that happens to contain challenge-like wording, or serves a
  challenge under a success status code: challenge detection judges the response body and shape,
  not the status code alone, and a false positive costs at most one unnecessary escalation.
- A host serves a challenge that no rung can pass: it is reported as permanently blocked and
  backs off to a long cooling-off period instead of being retried every run.
- A source's page shape changes at the same time as it starts blocking: the report distinguishes
  "blocked" from "read but unparseable" so the operator knows whether to fix a parser or a
  retrieval path.
- The real-browser rung fails to start, hangs, or leaks a process: the run reports it as
  unavailable, cleans up, and does not hold up other sources.
- Two hosts resolve to the same underlying platform, or one source spans several hosts: rung
  preference, credentials, budget, and cooling-off are tracked per host, and a block on one host
  does not silence the others.
- The system's browser identity is upgraded mid-roster: stored credentials tied to the old
  identity are discarded rather than presented under the new one.
- The operator overrides a cooling-off period repeatedly: the override is honored but the risk is
  stated, and the cooling-off period is not reset by the override.
- A source using the user's real account credentials is challenged: nothing is escalated,
  retried, or worked around; the user is told.

## Requirements *(mandatory)*

### Functional Requirements

**Browser fidelity**

- **FR-001**: System MUST present outbound retrieval requests as a current, real browser release
  would present them, including the full set of headers such a browser sends, in the order it
  sends them.
- **FR-002**: System MUST keep every part of its declared identity mutually consistent — browser
  version, platform, and capability claims MUST all describe the same real browser release.
- **FR-003**: System MUST match the connection-level characteristics of the browser it claims to
  be, not those of a generic programmatic client.
- **FR-004**: System MUST make its declared browser identity a single configured value used by
  every retrieval path, so no two paths disagree about who the system is.
- **FR-005**: System MUST persist per-host visitor state — cookies and any issued visitor
  credential — across runs and restarts, and MUST reuse it on subsequent visits to that host.
- **FR-006**: System MUST bind stored visitor state to the browser identity it was issued to, and
  MUST discard that state when the identity changes.
- **FR-007**: System MUST NOT share visitor state between hosts, and MUST NOT share it between a
  credentialed source and an anonymous one on the same host.
- **FR-008**: System MUST send a plausible navigation context on requests that a browser would
  only make after arriving from another page within the same site.
- **FR-009**: System MUST vary the interval between requests to a host rather than using a fixed
  delay.

**Escalation ladder**

- **FR-010**: System MUST provide an ordered set of retrieval methods, from a browser-fidelity
  direct request, through driving a real browser, to handing the page to the challenge-solving
  service already deployed with the system.
- **FR-011**: System MUST attempt the cheapest available method first and escalate only after
  detecting a challenge or refusal.
- **FR-012**: System MUST detect a challenge or refusal from the response's content and shape, not
  from its status code alone.
- **FR-013**: System MUST record, per host, the method that last succeeded, and MUST begin
  subsequent runs at that method.
- **FR-014**: System MUST periodically re-test a cheaper method for a host recorded as needing a
  heavier one, so no host is permanently pinned to an expensive path.
- **FR-015**: Users MUST be able to clear a host's recorded method preference and its stored
  visitor state.
- **FR-016**: System MUST report a page as blocked, with the reason, when every available method
  has been tried and challenged.
- **FR-017**: System MUST treat an unavailable heavier method — its service not running or not
  configured — as a blocked page with that stated reason, and MUST NOT fail the surrounding run.
- **FR-018**: System MUST NOT escalate for a source that authenticates with the user's own account
  credentials; such a source MUST report the block for manual resolution instead.
- **FR-019**: System MUST isolate any rendering of third-party pages from the rendering path used
  for the user's own generated documents.
- **FR-020**: System MUST make every retrieval method usable by every scraped source through one
  shared interface, so no source implements its own retrieval or its own challenge handling.

**Honest reporting**

- **FR-021**: System MUST NOT report a run as successful when its pages were blocked; a fully
  blocked run MUST be reported as blocked with a reason.
- **FR-022**: System MUST distinguish and report these outcomes per page: read successfully,
  challenged, refused, read but unparseable, and deferred for budget or cooling-off.
- **FR-023**: System MUST report a partially blocked run as partial, with the blocked-page count,
  while keeping the listings it read.
- **FR-024**: System MUST flag a source that returns zero listings after recent runs returned
  listings, even when no block was detected.
- **FR-025**: System MUST record, per host, the time and reason of the most recent block.

**Restraint and budget**

- **FR-026**: System MUST suspend contact with a host for a cooling-off period after a
  configurable number of consecutive blocked runs, and MUST lengthen that period on continued
  blocking.
- **FR-027**: Users MUST be able to override a cooling-off period for an on-demand run, and the
  system MUST state the risk and MUST NOT reset the period as a result.
- **FR-028**: System MUST honor a host's explicit instruction to wait a stated duration before
  waiting less than that duration.
- **FR-029**: System MUST honor a host's published crawl delay when it is slower than the
  system's own pacing.
- **FR-030**: System MUST enforce a configurable per-host daily request budget across all sources
  together, and MUST report requests beyond it as deferred rather than failed.
- **FR-031**: System MUST enforce per-host pacing across concurrently running sources, so several
  sources targeting one host cannot multiply its load.
- **FR-032**: System MUST NOT route retrieval through third-party proxy or scraping services,
  paid or free, and MUST NOT use anonymizing relay networks.

**Operator surface**

- **FR-033**: System MUST show, per scraped source and host, the retrieval method in use, the last
  block time and reason, and any cooling-off period in effect.
- **FR-034**: Users MUST be able to see when a host is deferring requests because its budget is
  exhausted, and when the budget resets.

### Key Entities

- **Retrieval Method**: One rung of the ladder — a way of fetching a page, with its cost and its
  availability. Ordered relative to the others.
- **Browser Identity**: The single coherent description of the browser the system presents itself
  as, covering its declared version, platform, capability claims, header set and order, and
  connection characteristics. Versioned, so stored visitor state can be tied to it.
- **Host Retrieval State**: Per-host memory: the method that last succeeded, stored visitor state
  and the identity it belongs to, consecutive block count, cooling-off expiry, last block time
  and reason, and requests used against the current budget period.
- **Page Outcome**: The classified result of one retrieval — read, challenged, refused,
  unparseable, or deferred — carrying its reason and the method used.
- **Run Verdict**: A run's aggregate honesty record: success, partial with a blocked-count, or
  blocked with a reason. Distinct from "zero listings found".

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: At least two sources that return nothing today return real listings on at least 90%
  of runs over a two-week period.
- **SC-002**: Zero runs report success while their pages were blocked, verified by tests that
  substitute challenge and refusal responses for real content.
- **SC-003**: A source that silently degrades to zero listings is flagged within one run of the
  degradation, with no operator action.
- **SC-004**: An operator can determine why a source failed — blocked, unparseable, cooling off,
  or out of budget — from the Sources screen alone, without reading logs, in 100% of failure
  cases.
- **SC-005**: Hosts that answer the cheapest retrieval method never trigger a heavier one, so
  routine runs cost no more than they do today.
- **SC-006**: For a host requiring escalation, the second and subsequent runs reach content
  without re-walking the ladder.
- **SC-007**: No host receives more requests per day than its configured budget, verified with
  every source enabled and running concurrently.
- **SC-008**: Zero requests are routed through any third-party proxy, scraping service, or
  anonymizing relay.
- **SC-009**: Zero account lockouts or credential invalidations occur on sources that use the
  user's real accounts, over three months of use.
- **SC-010**: A host that blocks the system repeatedly is contacted at a decreasing rate, and
  after sustained blocking is contacted no more than once per cooling-off period.
- **SC-011**: Adding a new scraped source requires no retrieval, challenge-handling, or
  cooling-off logic of its own.
- **SC-012**: No third-party page is ever rendered in the same context used to render the user's
  own documents.

## Assumptions

- Free-only is a hard constraint, and slowness is acceptable. Speed of discovery has no value
  here; being answered at all does. Every requirement is therefore free to prefer patience over
  throughput.
- The user's own residential connection is the system's single network origin and its best asset:
  clean, low-volume, and unremarkable. No IP rotation of any kind is in scope — free proxy pools
  and anonymizing relays are both widely pre-blocked by the very hosts this feature targets and
  would route the user's job-search activity through untrusted operators.
- The challenge-solving service is already present in the project's deployment definition and
  configured for by an existing setting, but is currently connected to no retrieval path. This
  feature wires up what already exists rather than introducing a new dependency. It remains
  optional: with it absent, the cheaper rungs still work and its absence is reported, not fatal.
- The per-host pacing already enforced on outbound requests is the foundation this builds on.
  Budget, cooling-off, jitter, and crawl-delay handling extend that existing mechanism rather
  than replacing it.
- Sources that already work are expected to keep working. Improved fidelity should be invisible
  to them, and any change in their behavior is a regression.
- Some hosts will remain unreachable by every free method — commercial bot-detection products
  exist precisely to win this contest. The measure of success is honest reporting of those hosts,
  not defeating them. Hosts of that class are candidates for a future, separate, explicitly
  opt-in approach and are out of scope here.
- Sources authenticating with the user's real accounts carry a qualitatively worse risk than the
  rest: a network block reverses, an account ban does not. They are deliberately excluded from
  escalation rather than given careful escalation.
- This feature changes only how pages are retrieved and reported. It adds no source, changes no
  scoring, and adds no auto-apply behavior of any kind.
- Feature 013 (employer board sources) is independent and deliberately needs none of this.
  Neither feature blocks the other, and 013 is expected to deliver more listings per unit of
  effort; this feature exists to stop the already-built scraped sources from rotting.
