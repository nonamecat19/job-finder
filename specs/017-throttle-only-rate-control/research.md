# Phase 0 Research: Throttle-Only Rate Control

**Feature**: `017-throttle-only-rate-control` | **Date**: 2026-07-28

## Purpose

Establish exactly what the daily-budget mechanism touches, what the pacing mechanism
actually does today, and how to close the gap between them so pacing can stand alone. The
spec assumed a couple of behaviours already existed; two of those assumptions turned out to
be wrong, and that is the most consequential finding here.

---

## Finding 1: The advertised crawl delay is recorded but never applied

**Status**: Spec assumption invalidated. Scope grows.

The spec's Assumptions section states "the system already records a per-host crawl delay
where one is advertised," and FR-009 requires the system to respect it. Recording is real;
respecting it is not implemented.

Evidence:

- `apps/api/internal/retrieval/state.go:204` defines `FetchAndSetCrawlDelay`, which fetches
  `robots.txt`, parses `Crawl-delay:`, and persists it to `host_retrieval_state.crawl_delay_seconds`.
- That function has **zero callers** in the codebase. Nothing ever populates the column
  outside of tests.
- `apps/api/internal/ratelimit/transport.go:46` exposes `PerHostRPS map[string]float64` as
  the per-host override hook. Nothing ever writes to it.
- `apps/api/internal/retrieval/transport.go` is three lines:
  `var DefaultTransport = ratelimit.NewTransport(nil)` — no base, no RPS, no overrides.

So today every third-party host is paced at exactly the same jittered `DefaultRPS = 0.7`,
and the crawl-delay column is dead data plumbed as far as the DTO
(`dto.HostRetrievalStatusDto.CrawlDelaySeconds`) and then never read by the dashboard either.

**Decision**: Implement crawl-delay-aware pacing as part of this feature rather than
deferring it. FR-009 is a stated requirement, the column and parser already exist, and
removing the daily cap makes per-host pacing the *only* protection — leaving the one signal
that hosts explicitly publish about their own tolerance unused would be the wrong trade
precisely when pacing carries the whole load.

**Rationale**: The alternative — shipping FR-009 as already-satisfied — would be false. The
alternative of dropping FR-009 from scope would weaken protection at the same moment the cap
disappears, contradicting SC-006 (block rate must not increase).

**Alternatives considered**:

- *Populate `PerHostRPS` once at server startup from the DB.* Rejected: crawl delays are
  discovered on first contact with a host, so a startup snapshot misses every host discovered
  later and needs a restart to take effect.
- *Leave crawl delay unwired, rely on the 0.7 RPS default alone.* Rejected per above; also
  leaves `FetchAndSetCrawlDelay` as permanent dead code.
- *Move pacing out of the transport into the retrieval service.* Rejected: the transport
  placement is deliberate and correct — it catches enrichment fetches and any adapter that
  uses the raw client, which a service-layer gate would miss.

---

## Finding 2: Rate resolution needs a DB-backed callback, not a static map

The pacing transport is an `http.RoundTripper` with no database access, by design. Crawl
delay lives in Postgres. Something has to bridge them without dragging a DB handle into the
`ratelimit` package.

**Decision**: Give `ratelimit.Transport` an optional resolver callback —
`RateResolver func(host string) (rps float64, source string, ok bool)` — plus a short TTL
cache on resolved values. `retrieval` wires a resolver backed by `StateStore`. The
`ratelimit` package keeps zero knowledge of Postgres.

**Rationale**: Keeps the existing per-host limiter map and its lazy construction intact;
adds one indirection at limiter-creation time only, not per request. The TTL cache means a
crawl delay discovered mid-session takes effect without a restart but does not put a query
in the hot path.

**Resolution precedence** (most conservative wins):

1. Explicit per-host operator override, if configured — always wins, including over a crawl
   delay, because it exists to encode knowledge the operator has and the board does not
   publish.
2. Advertised crawl delay of *N* seconds → `1/N` RPS, applied when it is **slower** than the
   default. A board advertising `Crawl-delay: 5` gets 0.2 RPS.
3. A crawl delay *faster* than the default is ignored; the conservative default stands.
   Boards that permit one request per second have not thereby asked for one.
4. No signal → `DefaultRPS`.

**Alternatives considered**: passing `*StateStore` directly into `ratelimit`. Rejected —
inverts the dependency and makes the package untestable without a database.

---

## Finding 3: Crawl-delay discovery needs a trigger

Since `FetchAndSetCrawlDelay` has no callers, wiring the resolver alone yields nothing.

**Decision**: Trigger discovery lazily from the retrieval service on first contact with a
host whose `crawl_delay_seconds` is still NULL, out of band from the fetch that triggered it
so no user-facing fetch waits on a `robots.txt` round trip. `FetchAndSetCrawlDelay` already
short-circuits when the value is set, so the guard is cheap.

**Rationale**: First contact is the only moment that both matters and is cheap. A background
sweep over all known hosts would be more machinery for no additional coverage.

**Note**: `parseCrawlDelay` returns `0` for absent/zero/negative delays and the setter stores
that `0`. Zero must be read as "asked and found nothing," never as "zero seconds between
requests" — the resolver must treat `0` as no signal and fall through to the default. This is
the one place where the existing sentinel choice is a live hazard for the new code.

---

## Finding 4: Full inventory of the daily-budget surface

Everything that must be removed, verified by direct search:

**Configuration**

- `apps/api/internal/config/config.go:114-116` — `PerHostDailyBudgetDefault`, tagged
  `PER_HOST_DAILY_BUDGET_DEFAULT`, comment cites "FR-030" from the feature that introduced it.
- `apps/api/internal/config/defaults.go:24` — default value `200`.
- No `.env.example`, compose file, or docs reference it. The knob was never documented, which
  is its own small argument that nobody was tuning it.

**Database**

- `apps/api/internal/db/migrations/00026_host_retrieval_state.sql:14-16` — columns
  `budget_period_start`, `budget_used`, `budget_limit INTEGER NOT NULL DEFAULT 200`.
- `apps/api/internal/db/queries/hostretrievalstate.sql` — three budget-only queries
  (`IncrementHostBudget`, `ResetHostBudget`, `DeductHostBudgetCheck`) plus budget columns
  threaded through `UpsertHostRetrievalState`.
- `apps/api/internal/db/sqlcgen/*` — generated; must be regenerated, never hand-edited.

**Go application code**

- `apps/api/internal/retrieval/state.go:108-200` — `IncrementBudget` (no callers),
  `CheckBudget` (~78 lines of period-rollover branching), `DeductBudget`.
- `apps/api/internal/retrieval/service_impl.go:87-118` — the gate itself: `CheckBudget`,
  the exhaustion `PageDeferred`, `DeductBudget`, and the "deduct race" `PageDeferred`.
- `apps/api/internal/retrieval/service_impl.go:277-283` — `HostStatus` budget fields.
- `apps/api/internal/retrieval/service.go:26-29` — `HostStatus` struct budget fields.

**Contract chain** (Constitution III applies at every hop)

- `apps/api/internal/dto/jobs.go:126-138` — `HostRetrievalStatusDto`, the tygo source of truth.
- `packages/shared/src/generated.ts:406-416` — tygo output; regenerate via `make tygo-generate`.
- `packages/shared/src/index.ts:382-391` — **a hand-maintained duplicate of the same
  interface.** See Finding 6.
- `packages/shared/dist/index.d.ts` — build output; refreshed by the shared package build.

**Dashboard**

- `apps/dashboard/src/features/sources/SourcesPage.tsx:171-176` — the "Budget: n/m" span and
  the "Budget resets:" span.

**Tests**

- `apps/api/internal/retrieval/budget_test.go` — despite the filename, contains only
  `TestParseCrawlDelay` and `TestCrawlDelayRe`. **No test covers the budget logic at all.**
  The file needs renaming to match its contents, not deleting.

**Decision**: Drop the columns rather than leaving them nullable-and-ignored. Per spec
Assumptions, historical usage counters carry no user value.

---

## Finding 5: What `PageDeferred` means after the change

`PageDeferred` currently has three producers, all in `service_impl.go`: cooling-off (line 78),
budget exhaustion (line 98), and the budget deduct race (line 110). Removing the budget leaves
cooling-off as the sole producer.

Downstream, three adapters (`jobgether.go:80`, `glassdoor.go:78`, `wellfound.go:82`) convert
`PageDeferred` into a Go `error` whose text contains "deferred", and
`apps/api/internal/ingestion/handler.go:453` lists `"deferred"` among its block markers, so
the run is recorded as blocked.

**Decision**: Keep `PageDeferred`, keep the adapter conversions, keep the block marker.

**Rationale**: With only cooling-off producing it, "deferred" now means "this host recently
refused us repeatedly and we are backing off" — which genuinely *is* a block-family outcome
and should be reported as one. FR-017 forbids reporting *pacing* as a failure; pacing never
produces an outcome at all, because `RoundTrip` blocks and then proceeds (FR-010). The two
concerns do not collide. Removing the marker would instead hide real trouble.

---

## Finding 6: Pre-existing violation of Constitution Principle III

`packages/shared/src/index.ts:382` hand-maintains `HostRetrievalStatusDto`, duplicating the
tygo-generated interface at `packages/shared/src/generated.ts:406`. Principle III states that
hand-maintained duplicate type definitions across apps are not permitted.

**Status**: Pre-existing, not introduced by this feature. Both copies must be edited in step
or the dashboard build will disagree with the API.

**Decision**: Update both copies, do not attempt the broader de-duplication here. Fixing the
duplication properly means re-pointing every consumer of `index.ts` at the generated module
and is a refactor with a blast radius far beyond this feature. Recorded in the plan's
Complexity Tracking so it is not mistaken for something this change endorsed.

---

## Finding 7: Migration numbering

Highest committed migration is `00026`. Two migrations exist uncommitted in the working tree:
`00027_drop_djinni_dashboard_subs.sql` and `00028_backfill_job_subscription.sql`.

**Decision**: This feature takes `00029`. Constitution requires unique, sequential goose
versions and forbids reuse, so the uncommitted files reserve their numbers even though they
belong to a different feature.

---

## Finding 8: Presentation of pacing (FR-012, FR-013, FR-016)

The existing panel renders three neutral facts in one `text-xs text-muted` row (rung, budget,
budget reset), then blocks in `text-warning` and cooling-off in `text-danger`. The visual
grammar for "routine fact" versus "problem" already exists and is exactly what FR-013 and
FR-015 ask for.

**Decision**: Replace the two budget spans with a pacing span in the same neutral row, and
phrase it in user terms plus the source of the rate — e.g.
`Pace: ~1 request every 5s (site-requested)` versus `Pace: ~1 request every 1.4s (default)`.

**Rationale**: FR-016 requires terms a non-technical user can act on. A bare `0.2 RPS` fails
that. An interval in seconds is directly meaningful, and naming the source answers the
obvious follow-up question ("why so slow?") without a support round trip, which is what
SC-003 measures.

**Decision**: Expose both the raw rate and its source in the DTO, and format for display in
the dashboard. Formatting belongs on the presentation side; the contract carries data.

---

## Open questions

None. No `NEEDS CLARIFICATION` markers remain in the Technical Context.
