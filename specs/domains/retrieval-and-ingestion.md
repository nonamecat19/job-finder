# Domain: Retrieval & Ingestion

Consolidates **014** browser-fidelity fetch ladder, **017** throttle-only rate control,
**025** batched atomic ingest persistence, and **043**'s relocation of the fetch stack into
the scraper library.

Implementation: **the fetch machinery is in the `github.com/nonamecat19/jobscraper`
library** (`retrieval/`, `scraping/`) after 043 — see
[`codebase-structure.md`](codebase-structure.md) § 5. What remains app-side:
`apps/api/internal/retrieval/` (the `StateStorePort` implementation and the engine wiring)
and `internal/jobsources/` (ingest persistence). `internal/ratelimit/` and
`internal/platform/scraping/` no longer exist — both were vendored into the library. How it
works:
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

The three rungs, cheapest first, are `direct` → `browser` → `flaresolverr`.

### 2.1 `retrieval.Service` — the seam every adapter uses

`jobscraper/retrieval` (was `apps/api/internal/retrieval` before 043). Adapters call this
instead of `scraping.Scraper.FetchHTML` or `HTTPClient()` directly; that indirection *is*
014-FR-020.

```go
type Service interface {
    Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
    HostStatus(ctx context.Context, host string) (HostStatus, error)
    ClearRungPreference(ctx context.Context, host string) error
    ClearCookies(ctx context.Context, host string) error
    OverrideCoolingOff(ctx context.Context, host string) (remaining time.Duration, err error)
}

type FetchRequest struct {
    URL             string
    Headers         map[string]string // merged *under* the browser identity's own set
    UsesUserAccount bool              // true ⇒ never escalate past `direct` (014-FR-018)
    RefererPage     string            // the page this request should appear to follow (014-FR-008)
}
```

**The load-bearing rule: a block is a return value, never a Go error.** `Fetch` returns a
`FetchResult` whose `Outcome.Status` carries `Read` / `Challenged` / `Refused` / `Deferred`
with `err == nil`. A non-nil error means something *operational* broke — a malformed URL, a
cancelled context — not "the host said no". Callers MUST branch on `Outcome.Status`; a naive
`if err != nil` must not be able to mistake a block for a crash, or a crash for a block.
`Body` is populated only when the status is `Read`.

Behaviour the tests pin, not just the types:

| Given | `Fetch` must |
|---|---|
| `currentRung = "browser"` for the host | Start at `browser`, not `direct` (014-FR-013). |
| A `direct` attempt returns `Challenged` | Retry the same URL at the next rung **within the same call** — the caller never re-invokes to escalate. |
| Every rung through `flaresolverr` returns `Challenged`/`Refused` | Return that outcome with `err == nil`. |
| `flaresolverr` unconfigured or failing its health check, and the ladder wants it | Return `Deferred` (blocked, with that stated reason) and **not** a Go error that would fail the surrounding run (014-FR-017). |
| `UsesUserAccount = true` and `direct` returns `Challenged`/`Refused` | Return immediately without trying `browser` or `flaresolverr` (014-FR-018). |
| The host is cooling off and `OverrideCoolingOff` was not called | Return `Deferred` **without making any network request**. |
| A successful `Read` at any rung | Persist that rung as `currentRung` and reset `consecutiveBlocks` to 0 before returning. |

`OverrideCoolingOff` (014-FR-027) lets one on-demand run bypass an active cooling-off window
**without resetting its expiry**, and returns the remaining duration so the caller can show
the operator what risk they are taking.

### 2.2 Host state crosses a port (043)

The engine no longer reaches a database. It reads and writes host state — rung preference,
crawl delay, consecutive blocks, cooling-off window, cookies — through
`retrieval.StateStorePort`, a 9-method interface in library-owned types (043-FR-007). The
app's `retrieval.StateStore` implements it against `sqlcgen`, and **the encryption stays on
the app side of that line**: cookies cross the port as plaintext JSON and are encrypted with
`internal/crypto` and `CONFIG_ENCRYPTION_KEY` before they reach Postgres (043-FR-012). The
library never sees the key.

Two behaviours the port's shape pins:

- `Get` returning `(nil, nil)` for an unknown host is **legal** and means "no history" — the
  engine starts at the cheapest rung. It is not an error case.
- `*time.Time`, not `pgtype.Timestamptz`, carries every timestamp. A nil pointer is "never
  happened"; the app does the `pgtype` translation in its own field-copy.

Construction is `NewEngine(identity, store, EngineOpts{...})`. `EngineOpts` carries
`BrowserEnabled`, `FlaresolverrURL`, `CheapRungRetestInterval`, `CoolingOffThreshold` and
`CoolingOffBaseDuration` as **plain values** — the library reads no config file and no
environment (043-FR-008). `apps/api/internal/retrieval.NewService` is the one place that maps
`config.Config` onto those fields. A nil identity selects the library's default browser
identity, so a consumer with no identity of its own still gets 014-FR-002 fidelity.

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

### 3.1 Rate resolution

The paced transport lives in `jobscraper/retrieval/pacing.go` after 043 (vendored from the
deleted `internal/ratelimit`, 043-FR-008). `Transport` resolves a host's pace through an
injected seam:

```
RateResolver func(host string) (rps float64, source string, ok bool)
```

- Optional. A nil resolver gives every host `DefaultRPS` — which is why the pre-017
  `ratelimit` tests still pass untouched.
- Consulted at **limiter construction**, then cached for a short TTL. Pacing must never put a
  query in the hot path.
- `ok == false` falls through to `DefaultRPS`.
- **The pacing code must not import `db` or any Postgres type.** Since 043 it reads host
  state through `StateStorePort` like the rest of the engine, and the resolver is still
  injected by the composition layer.

Precedence:

1. Operator override → `override`.
2. `crawl_delay_seconds = N > 0` **and** `1/N` slower than the default → `site-requested` at
   `1/N`.
3. Otherwise → `default` at `DefaultRPS`.

`crawl_delay_seconds == 0` means "we asked, the host advertised nothing" and falls to case 3.
It must never resolve to an unbounded rate.

> **Defect — the resolver is not wired in the running app.** `DefaultTransport` is
> constructed as `NewTransport(nil)`, and its `RateResolver` is only set by
> `ConfigureDefaultTransport`, which **nothing calls** outside tests
> (`apps/api/internal/retrieval/transport.go` exposes it; no composer invokes it). So
> precedence cases 1 and 2 never fire at runtime: every host is paced at `DefaultRPS`, an
> advertised `crawl_delay_seconds` is stored but not honoured (**017-FR-009 is unmet in
> practice**), and `HostStatus.Pacing.Source` always reports `default`. `RateFor` still reads
> that same unconfigured transport (`retrieval/engine.go:280`), so § 3.2's status view is
> consistent with the code and wrong about the world.
>
> **This predates 043.** The call existed in `platform.go` when 017 shipped (`4376e68`) and
> was dropped by the composition-root extraction (`1e613fd`); the extraction only relocated
> the already-unwired code. There is also no `HostRPSOverrides` key in `internal/config`, so
> operator overrides have no input path either.
>
> Fixing it is one call in `composeRetrieval` after the state store exists —
> `retrieval.ConfigureDefaultTransport(stateStore, nil)` — plus a config key if overrides are
> wanted. Recorded here rather than fixed silently, per § "record, don't drop" practice.

Jitter is ±25% and is **deliberately not exposed** anywhere. It is anti-fingerprinting
machinery; surfacing a fluctuating number would undercut 017-FR-016's requirement that the
displayed pace be something a user can act on.

Pacing produces no `PageOutcome` at all. After 017, `PageDeferred` has exactly one producer —
cooling-off — and is still reported as a block, because it now unambiguously *is* one.

### 3.2 `GET /api/hosts/{host}/retrieval-status`

`internal/httpapi/hosts.go`; the DTO originates in `internal/dto/jobs.go` and reaches the
dashboard through tygo into `packages/shared/src/generated.ts`. 404 when the host has no
state row yet (never contacted).

```jsonc
{
  "host": "djinni.co",
  "identityVersion": "v3",
  "currentRung": "direct",          // direct | browser | flaresolverr
  "lastBlockAt": "2026-07-27T09:14:02Z",
  "lastBlockReason": "challenged via direct",
  "coolingOffUntil": null,
  "crawlDelaySeconds": 5,           // raw advertised value; 0 = asked, nothing advertised
  "pacing": {
    "requestsPerSecond": 0.2,
    "intervalSeconds": 5,
    "source": "site-requested"      // default | site-requested | override
  }
}
```

017 **removed** `budgetUsed`, `budgetLimit` and `budgetResetsAt` (017-FR-014) with no
deprecation window — the dashboard is the only consumer and ships from this repo. `pacing` is
always present, because 017-FR-007 leaves no unpaced third-party host. Both
`requestsPerSecond` and `intervalSeconds` are sent although either derives from the other:
the rate is the machine-meaningful figure, the interval is what the UI shows, and deriving in
the client invites two clients rounding differently.

`lastBlockReason` must never cite an exhausted allowance (017-FR-017).

**Presentation contract** — binding on the dashboard, not on the wire. The difference between
"normal" and "problem" is the entire point of 017:

| Field | Treatment |
|---|---|
| `pacing` | Neutral (`text-muted`), on the same row as `currentRung` — a routine operational fact |
| `lastBlockAt` / `lastBlockReason` | Warning (`text-warning`) — a genuine host refusal |
| `coolingOffUntil` | Danger (`text-danger`) — active back-off, the host is untouchable |

Pacing MUST NOT carry error, warning or danger styling and MUST NOT get an alert icon. It
shows whether or not anything is wrong, exactly like the current rung. Display form:
`~1 request every 5s (site-requested)`.

### 3.3 Operator actions and run verdicts

| Method | Path | Result |
|---|---|---|
| `POST` | `/api/hosts/{host}/clear-rung-preference` | `204`, idempotent. Resets `currentRung` to the cheapest rung so the next run re-tests (014-FR-015). |
| `POST` | `/api/hosts/{host}/clear-cookies` | `204`, idempotent. Discards stored visitor state (014-FR-015). |
| `POST` | `/api/hosts/{host}/override-cooling-off` | `{ remainingSeconds }`. Applies the override and states the remaining risk; the stored `coolingOffUntil` is **unchanged** by the call. |

Recent-run rows carry the verdict inline rather than through a new endpoint —
`{ verdict: "success" | "partial" | "blocked", blockedCount, blockReason }` — merged into the
existing recent-runs response the Sources screen's `RecentRunsPanel` already consumes
(014-FR-033).

**Pacing never fails a request:**

| Situation | Required behaviour |
|---|---|
| Token unavailable | Block until available, then proceed (017-FR-010) |
| Context cancelled while waiting | Return promptly — shutdown is not held up |
| Loopback / localhost destination | Not paced at all |
| Run delayed only by pacing | Reported `success` (017-FR-018) |
| Any user-visible reason string | Never cites pacing or an allowance as a failure (017-FR-017) |

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

### 5.1 The schema 025 needed

Migration `00032_batch_ingest.sql` added exactly two things:

- **`Job.lastSeenRunId uuid`** — nullable, no default, no backfill. `RecordJobRepost` used to
  increment `seenCount` unconditionally, so a retry after a mid-run failure re-counted every
  posting the failed attempt had already stored, inflating the repost signal that ghost-job
  scoring reads. NULL is a correct initial state: pre-existing rows have no tracked run, and
  the guard uses `IS DISTINCT FROM`, which handles NULL correctly.
- **`CREATE INDEX "Job_lower_company_idx" ON "Job" (LOWER("company"))`** — a *functional*
  index, required because the merge-candidate predicate is on `LOWER("company")`, not
  `"company"`. Without it every board-vendor posting scanned the whole table (025-FR-007).

`CONCURRENTLY` was deliberately not used: it cannot run inside goose's transaction, and the
plain `ACCESS EXCLUSIVE` lock is harmless here — single-user deployment, small `Job` table,
migrations run at startup before the API serves traffic. `ADD COLUMN` with no default is
metadata-only on PostgreSQL 11+, so it is instant. Rolling back loses `lastSeenRunId` values
only — it restores the double-counting bug but loses no posting data.

**Verifying 025-FR-007 means checking the planner, not the catalogue.** An index that exists
but is never chosen does not satisfy the requirement:

```sh
psql "$DATABASE_URL" -c 'EXPLAIN SELECT id FROM "Job" WHERE LOWER("company") = LOWER($$Acme$$);'
# expect: Index Scan using "Job_lower_company_idx" — NOT Seq Scan
```

On a table small enough that a sequential scan legitimately wins, `SET enable_seqscan = off`
confirms the index is *usable*, then re-check on a seeded table that it is *chosen*.

### 5.2 The batched query set

Six statements in `internal/db/queries/`, generated through sqlc (`make sqlc-generate`; never
hand-edit `internal/db/sqlcgen/`). The per-posting originals — `GetJobByDedupeKey`,
`RecordJobRepost`, `InsertJob`, `FindJobByCompany`, `MergeJobBoard` — are **retained**, not
deleted; other call sites and tests still use them. Only the ingest persist path stopped
calling them.

| Query | Role | Guarantee the caller depends on |
|---|---|---|
| `GetJobsByDedupeKeys` | Classify known vs new in one round trip | At most one row per input key (backed by `Job_dedupeKey_unique`). Absent keys are simply not returned — **never assume input/output alignment by position.** |
| `FindJobsByCompanies` | Batched merge-candidate lookup: `DISTINCT ON (LOWER("company"))`, most recent job from a *different* source | At most one row per company, correlated by the returned lowercased `company_key`, not by position |
| `BulkInsertJobs` | One `INSERT … SELECT FROM unnest(...) ON CONFLICT ("dedupeKey") DO NOTHING RETURNING` per chunk | `RETURNING` contains **exactly** the rows this statement inserted. A key absent from the result was already present or lost a race — either way the caller must not queue downstream work for it. Row order is not guaranteed; correlate by `dedupeKey`. |
| `BulkRecordJobReposts` | Set-based repost update plus the retry guard | Idempotent per run: running it twice with the same run id increments each posting exactly once. Returns the affected row count, which is the run's *true* repost total. |
| `BulkMergeJobBoards` | Batch fold of board postings into an existing job | The three arrays are position-aligned and must be built in lockstep |
| `BulkInsertActivities` | Batch form of the per-task activity insert | Correlate by `(jobId, kind)` — a single job gets two rows (`match` and `ghost`), so `jobId` alone is not a key |

Three details that look like style but are correctness:

- **`ON CONFLICT DO NOTHING` is why `COPY` was rejected** for the bulk insert. `COPY` cannot
  express it, and it is what makes both in-batch duplicates and genuine races with a
  concurrent run harmless — the loser simply gets no `RETURNING` row, queues nothing
  downstream, and does not fail (025-FR-013).
- **`lastSeenRunId IS DISTINCT FROM $3`, not `!= $3`.** The column is NULL for every row
  predating 00032, and `NULL != $3` evaluates to NULL, which would exclude every pre-existing
  row from ever being counted.
- **Nullable columns must carry SQL NULL in the arrays, not empty strings.** `sqlc.yaml` sets
  `emit_pointers_for_null_types: true`, so the Go side builds `[]*string` / `[]*time.Time`
  for `externalId`, `location`, `salaryRaw`, `description` and `postedAt`.

`titlesOverlap` stays in Go (`dedupe.go`) rather than moving into SQL: the query returns
candidates, Go decides whether each is a match. Reimplementing a tested word-overlap
heuristic in an untested place is not a batching win.

**Interaction budget.** Only `GetJobsByDedupeKeys` runs unconditionally; the other five are
conditional on the chunk containing board postings, new postings, known postings, resolved
merges, or inserted postings respectively. **Maximum 6 statements per chunk against
025-SC-002's budget of 10** — and constant in the posting count, which is the actual
requirement.
