# Domain: Retrieval & Ingestion

Consolidates **014** browser-fidelity fetch ladder, **017** throttle-only rate control,
**025** batched atomic ingest persistence.

Implementation: `apps/api/internal/platform/scraping/`, `internal/retrieval/`,
`internal/ratelimit/`, `internal/jobsources/`. How it works:
[`docs/ingestion/retrieval-and-fetching.md`](../../docs/docs/ingestion/retrieval-and-fetching.md),
[`docs/ingestion/rate-limiting.md`](../../docs/docs/ingestion/rate-limiting.md).

This domain is shared infrastructure. **No job source implements its own retrieval,
pacing, challenge handling or persistence** (014-FR-020, 014-SC-011) — adding a source
requires zero work here.

---

## 1. Browser fidelity (014-FR-001..008)

Outbound requests present as a *current, real browser release*, consistently:

- **014-FR-002**: every part of the declared identity agrees with every other part — user
  agent, client hints, header order, accepted languages and encodings. A mismatched set is
  more detectable than an honest one.
- **014-FR-003**: connection-level characteristics match the browser being claimed.
- **014-FR-004**: the declared identity is **one configured value** used by every request
  path. Not per-adapter, not per-call.
- **014-FR-005/006/007**: per-host visitor state (cookies, issued visitor tokens) is
  persisted, bound to the identity it was issued to, and re-issued when that identity
  changes. State is never shared between hosts, and never between an authenticated session
  and an anonymous one.
- **014-FR-008**: requests that a browser would only make after a navigation carry a
  plausible navigation context.

## 2. The escalation ladder (014-FR-010..019)

An ordered set of retrieval methods, cheapest first, from a browser-fidelity HTTP client up
to a real browser engine.

| Rule | Requirement |
|---|---|
| 014-FR-011 | Try the cheapest available method first; escalate **only** after the cheaper one demonstrably fails. |
| 014-FR-012 | Detect a challenge or refusal from the response's content and shape — not from status code alone. |
| 014-FR-013 | Record, per host, the method that last succeeded; begin subsequent runs at that method. |
| 014-FR-014 | Periodically re-test a cheaper method for a host pinned to a heavier one, so a host never stays permanently expensive after it stops challenging. |
| 014-FR-015 | A user can clear a host's recorded method preference and its stored visitor state. |
| 014-FR-016 | When every available method fails, report the page as blocked **with the reason**. |
| 014-FR-017 | A heavier method whose service is not running is *unavailable*, not *failed* — a distinct, reported condition. |
| 014-FR-018 | Never escalate for a source that authenticates with the user's own account. Protecting the user's account beats reading the page (014-SC-009: zero account lockouts). |
| 014-FR-019 | Rendering third-party pages is isolated from the rendering path used for the user's own content (014-SC-012). |
| 014-FR-032 | **Never** route retrieval through a third-party proxy or scraping service (014-SC-008: zero such requests). This is a hard boundary, consistent with Constitution V. |

Cost discipline: 014-SC-005 — hosts that answer the cheapest method never trigger a heavier
one. 014-SC-006 — for a host that does need escalation, the second and subsequent runs reach
content at the recorded method without re-walking the ladder.

## 3. Pacing — throttle only (017, revising 014)

**017 revoked 014-FR-030 in full.** 014 originally specified a configurable per-host *daily
request budget*. 017 removed the concept: there is no cap on how many requests a host may
receive in a day, only on how fast they arrive.

Binding rules:

| # | Requirement |
|---|---|
| 017-FR-001 | No fixed maximum number of requests per host per period. |
| 017-FR-002 | Never refuse, defer or skip a fetch on the grounds of how many requests already went to that host. |
| 017-FR-003 | Per-host cumulative request counts and their reset windows are not tracked at all. |
| 017-FR-004 | The operator-facing default-budget setting is removed. |
| 017-FR-005 | Pre-existing stored per-host retrieval state stays valid across the change. |
| 017-FR-006/007/008 | Space outbound requests per destination **host** at a minimum interval; this applies to every outbound third-party request regardless of which source issued it, so several sources hitting one host share one pace. (Supersedes 014-FR-031.) |
| 017-FR-009 | A host's advertised crawl delay is respected when it is slower than the configured pace. (Retains 014-FR-029.) |
| 017-FR-010 | A request waiting for its pacing turn **waits and then proceeds** — it is never dropped. |
| 017-FR-011 | Block-triggered cooling-off is retained: a host is paused after a block. (Retains 014-FR-026, 014-FR-028 host-requested waits, 014-SC-010 decreasing contact rate under repeat blocks.) |
| 017-FR-018 | A run whose only notable event was waiting for pacing is reported as a **success**, not as partial or deferred. |

Presentation (017-FR-012..017):

- The host retrieval status view shows the pace currently in force, as ordinary
  informational status.
- It MUST NOT display any quota, budget, allowance or reset time — the concepts no longer
  exist, and showing them would be a lie (017-SC-004).
- Genuine blocks and active cooling-off stay **visually distinct** from ordinary pacing.
- Pacing is expressed in terms a non-technical user can act on.
- No user-visible outcome, reason string or log may describe a limit that is not enforced
  (017-FR-017).

017-FR-019 requires project documentation describing IP-ban avoidance to match this model.
017-FR-020 required removing the daily-limit tests rather than leaving them asserting dead
behaviour.

## 4. Outcome reporting (014-FR-021..025, 033, 034)

- A run whose pages were blocked is **never** reported successful (014-SC-002). A fully
  blocked run fails; a partially blocked run is *partial*, with the blocked-page count and
  reason.
- Per-page outcomes are distinguished and reported: read successfully, blocked, unparseable,
  unavailable.
- A source returning zero listings after recent runs returned listings is flagged within one
  run (014-FR-024, 014-SC-003) — silent degradation to zero is the failure mode that
  scraping hides best.
- Per host: the time and reason of the most recent block are recorded (014-FR-025), and the
  Sources screen shows the retrieval method in use and last block per source/host
  (014-FR-033).
- 014-SC-004 is the operator-facing bar: an operator can tell **why** a source failed —
  blocked, unparseable, cooling off, unavailable — from the UI alone.

## 5. Batched atomic persistence (025)

Storing a run's results is one bounded, atomic unit — not a statement per posting.

**Batching**

| # | Requirement |
|---|---|
| 025-FR-001 | Deciding which postings are already known uses a number of DB interactions that does **not** grow with the result count. |
| 025-FR-002/003 | New postings are created in bulk; repeat sightings are recorded in bulk. |
| 025-FR-006 | Employer-board merge resolution (see `job-sources.md` § 3) is decided for the whole batch at once. |
| 025-FR-007 | The company-name match lookup is served by an index, never a full scan of the job table. |
| 025-FR-008 | Duplicate identities *within* one run's results collapse to one posting before storage. |
| 025-FR-009 | Batches over a configured maximum split into bounded chunks, preserving the all-or-nothing outcome across the whole run. |

**Atomicity**

- 025-FR-004: a failure at any point leaves **none** of that run's postings recorded. No
  partial batches (025-SC-004: 100 randomised failure-injection trials, all-or-nothing every
  time).
- 025-FR-005: a posting's repeat-sighting count rises by at most one per source run, however
  many times that run is attempted (025-SC-003: ten forced mid-storage failures and retries
  still yield exactly one increment).
- 025-FR-013: concurrent runs storing the same posting produce exactly one posting, with
  neither run failing.
- 025-FR-012: identity classification is unchanged from before the batching work, so
  previously recorded postings are still recognised.

**Downstream and observability**

- 025-FR-010: each newly stored posting is queued for **exactly one** downstream activity —
  scoring plus ghost checking for full postings, detail retrieval for stubs. Not both, not
  neither.
- 025-FR-011: found/new totals describe the successful attempt only.
- 025-FR-014: per run, storage duration and DB interaction count are recorded, so the
  improvement is observable rather than asserted (025-SC-002: interaction count is constant
  in the posting count, apart from chunking).
- 025-SC-005: the stranded-job recovery sweep must not start finding *more* work after this
  change — a rise means atomicity regressed.
