# Implementation Plan: Throttle-Only Rate Control

**Branch**: `017-throttle-only-rate-control` | **Date**: 2026-07-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/017-throttle-only-rate-control/spec.md`

## Summary

Remove the per-host daily request budget entirely and make per-host request pacing the single
rate-control mechanism, presented to users as ordinary operational status rather than as a
failure.

The removal is mechanical: one config knob, three DB columns, three sqlc queries, ~100 lines
of budget logic in `retrieval`, three DTO fields, and two dashboard spans. The substance of
the work is elsewhere. Phase 0 research established that the advertised crawl delay — which
the spec assumed was already honoured — is recorded but **never read**: `FetchAndSetCrawlDelay`
has no callers and `ratelimit.Transport.PerHostRPS` is never populated. Every third-party host
is currently paced at the same flat 0.7 RPS. Since removing the cap makes pacing the only
protection, FR-009 becomes real implementation work: wire crawl delay into the pacing rate via
a DB-backed resolver callback, and trigger discovery on first contact with a host.

Presentation follows the visual grammar the panel already uses — neutral muted text for
routine facts (rung, and now pacing), warning for genuine blocks, danger for cooling-off.
Cooling-off is deliberately retained; it reacts to observed refusals, not to volume.

## Technical Context

**Language/Version**: Go 1.23+ (`apps/api`), TypeScript 5 / React 18 (`apps/dashboard`)

**Primary Dependencies**: `golang.org/x/time/rate` (pacing), `pgx/v5` + sqlc (typed DB access),
goose (migrations), tygo (Go DTO → TS), chi (HTTP routing), TanStack Query + Tailwind (dashboard)

**Storage**: PostgreSQL — table `host_retrieval_state`, losing three columns via migration `00029`

**Testing**: `go test` (`make test-go`), vitest (`make test-react`), Docker-backed
`make test-integration`; cross-app gate `make test-lint`

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Web application — Go API + React dashboard + shared TS types package

**Performance Goals**: Per-host outbound rate stays at or below the conservative pace
(`DefaultRPS = 0.7`, jittered ±25%, burst 2) including under concurrent runs against one host.
Rate resolution must not add a database query to the per-request path — resolve at limiter
construction with a short TTL.

**Constraints**: No fixed request cap per host per any calendar period. Pacing blocks and
proceeds; it never fails a request or produces a `PageOutcome`. Existing `host_retrieval_state`
rows stay valid across the migration with no operator intervention. Generated code
(`sqlcgen`, `generated.ts`) is regenerated, never hand-edited.

**Scale/Scope**: ~20 job-board hosts; three apps touched (`apps/api`, `apps/dashboard`,
`packages/shared`); one migration; one breaking response-shape change with a single in-repo
consumer.

## Constitution Check

*GATE: evaluated before Phase 0, re-evaluated after Phase 1 design.*

| Principle | Verdict | Assessment |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ PASS | Touches only how pages are fetched and paced. No path to application submission or employer contact is added, altered, or approached. |
| **II. Grounded Generation** | ✅ PASS | No LLM-generated content involved. Removing the cap changes how many postings are retrieved, never how they are described. |
| **III. Typed Contracts Across Service Boundaries** | ⚠️ PASS with noted pre-existing violation | `HostRetrievalStatusDto` changes shape, so the full chain must regenerate: `dto/jobs.go` → `make tygo-generate` → `generated.ts`; DB changes → `make sqlc-generate` → `sqlcgen`. Both `make sqlc-check` and `make tygo-check` must pass. **However**: `packages/shared/src/index.ts:382` hand-maintains a duplicate of this exact interface, which Principle III forbids. Pre-existing, not introduced here. Both copies get edited in step; see Complexity Tracking. |
| **IV. Test Discipline Per Language** | ✅ PASS | Change crosses `apps/api` + `apps/dashboard` + `packages/shared`, so `make test-lint` is required before done. Integration coverage for the no-cap behaviour uses real Postgres via `make test-integration`, not mocks. Note: no test currently covers the budget logic being removed (research Finding 4), so removal is not protected by existing tests — new pacing tests must land alongside. |
| **V. Local-First, Self-Hosted** | ✅ PASS | No new external dependency. Adds one `robots.txt` fetch per host on first contact, out of band. Job-board hosts are discovery sources, not inference. |

**Migration discipline**: highest committed migration is `00026`; `00027` and `00028` exist
uncommitted in the working tree for a different feature. This feature takes **`00029`** —
numbers are never reused, even when the reserving migration is uncommitted.

**Gate result**: PASS. One pre-existing violation documented rather than silently inherited.

**Post-Phase-1 re-check**: PASS. Design introduces no new cross-boundary hand-maintained types.
The `RateResolver` seam keeps `ratelimit` free of any database import, so the new dependency
runs composition → `retrieval` → `ratelimit` and never inverts.

## Project Structure

### Documentation (this feature)

```text
specs/017-throttle-only-rate-control/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── host-retrieval-status.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 — created by /speckit-tasks, NOT by this command
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── ratelimit/
│   │   ├── transport.go            # + RateResolver callback, TTL cache, RateFor(host)
│   │   └── transport_test.go       # + resolver precedence, concurrency, loopback exemption
│   ├── retrieval/
│   │   ├── transport.go            # wire resolver into DefaultTransport (currently 3 lines)
│   │   ├── state.go                # − IncrementBudget/CheckBudget/DeductBudget; crawl-delay reader
│   │   ├── service.go              # HostStatus: − budget fields, + pacing
│   │   ├── service_impl.go         # − budget gate (l.87-118); + lazy crawl-delay discovery
│   │   ├── budget_test.go          # → rename to crawldelay_test.go (holds only crawl-delay tests)
│   │   └── pacing_test.go          # new: resolution precedence incl. the 0-means-no-signal case
│   ├── config/
│   │   ├── config.go               # − PerHostDailyBudgetDefault
│   │   └── defaults.go             # − PER_HOST_DAILY_BUDGET_DEFAULT
│   ├── dto/jobs.go                 # HostRetrievalStatusDto: − 3 budget fields, + pacing
│   └── db/
│       ├── migrations/00029_drop_host_budget.sql   # new
│       ├── queries/hostretrievalstate.sql          # − 3 budget queries; upsert loses 3 cols
│       └── sqlcgen/                                # REGENERATED, never hand-edited
├── cmd/server/
│   └── compose_types.go            # hostsAdapter: map pacing instead of budget
│
apps/dashboard/src/features/sources/
└── SourcesPage.tsx                 # replace 2 budget spans with neutral pacing line

packages/shared/src/
├── generated.ts                    # REGENERATED via make tygo-generate
└── index.ts                        # hand-maintained duplicate — edit in step (see Complexity)
```

**Structure Decision**: Existing web-application layout — Go API, React dashboard, shared TS
types. No new packages or directories. Pacing stays in `internal/ratelimit` as an
`http.RoundTripper`; that placement is deliberate and load-bearing, since it catches
enrichment fetches and any adapter using the raw client, which a service-layer gate would miss.

## Implementation Phases

Ordered so the system is coherent at each step. Removal lands before the replacement is
strengthened, because the removal is what the user asked for and it stands alone.

### Stage A — Remove the daily budget (User Story 1, P1)

Delivers the whole of US1 by itself.

1. Migration `00029`: drop `budget_period_start`, `budget_used`, `budget_limit`. `Down`
   re-adds them with original types and defaults.
2. Remove the three budget queries from `hostretrievalstate.sql`; drop the three columns from
   `UpsertHostRetrievalState`. Run `make sqlc-generate`.
3. Delete `IncrementBudget`, `CheckBudget`, `DeductBudget` from `state.go` (~100 lines,
   including `CheckBudget`'s period-rollover branching). Drop budget fields from the upsert
   params.
4. Delete the budget gate at `service_impl.go:87-118` — both `PageDeferred` returns go with it.
   The cooling-off check immediately above stays untouched.
5. Remove `PerHostDailyBudgetDefault` from `config.go` and its default from `defaults.go`.
6. Verify: exceeding 200 requests to one host in one session refuses nothing.

### Stage B — Make pacing carry the load (FR-006 – FR-010, closes research Finding 1)

The gap the spec assumed was already closed.

7. Add `RateResolver func(host string) (rps float64, source string, ok bool)` to
   `ratelimit.Transport`, consulted at limiter construction with a short TTL cache. Nil
   resolver ⇒ current behaviour, so existing tests pass untouched.
8. Add `RateFor(host)` so the API can report the effective rate without issuing a request.
9. Wire a `StateStore`-backed resolver into `retrieval.DefaultTransport`, applying the
   precedence in [data-model.md](data-model.md#resolution-rules): override → site-requested
   (only when slower than default) → default.
10. Trigger `FetchAndSetCrawlDelay` on first contact with a host whose `crawl_delay_seconds`
    is `NULL`, out of band so no user-facing fetch waits on `robots.txt`.
11. **Handle `0` correctly**: `parseCrawlDelay` returns `0` for absent, malformed, zero, and
    negative values alike. `0` means "asked, nothing advertised" and must fall through to the
    default — never to an unbounded rate. This is the single highest-risk line in the feature.

### Stage C — Present pacing as normal (User Story 2, P2)

12. `dto/jobs.go`: drop `budgetUsed`/`budgetLimit`/`budgetResetsAt`, add the `pacing` object.
    Run `make tygo-generate`. Hand-edit the `index.ts` duplicate to match.
13. `retrieval.HostStatus` + `hostsAdapter`: populate pacing from `RateFor`.
14. `SourcesPage.tsx`: replace the two budget spans with a pacing line in the same neutral
    muted row as `Rung:` — `Pace: ~1 request every 5s (site-requested)`. No alert icon, no
    warning or danger styling. Block and cooling-off lines keep theirs (FR-015).

### Stage D — Remove leftover surfaces (User Story 3, P3)

15. Rename `budget_test.go` → `crawldelay_test.go`. It contains only `TestParseCrawlDelay` and
    `TestCrawlDelayRe` — no budget test has ever existed. Rename, do not delete.
16. Add pacing tests: resolver precedence, the `0` case, concurrent callers sharing one
    limiter, loopback exemption, and an integration case exceeding the old cap.
17. Sweep for residue per [quickstart.md](quickstart.md) Scenario 6; update any documentation
    describing rate control to name pacing and cooling-off as the mechanisms.
18. `make sqlc-check`, `make tygo-check`, `make test-lint`.

## Key Risks

| Risk | Mitigation |
|---|---|
| **Removing the cap raises the real block rate** — the cap may have been incidentally protecting some host. | SC-006 measures block/cooling-off rate per host after the change. Stage B strengthens pacing in the same release rather than leaving the default flat. Cooling-off remains as the backstop. |
| **`crawl_delay_seconds == 0` read as "no delay"** — would produce an unbounded rate on hosts that were asked and answered nothing, i.e. most of them. | Called out in research Finding 3, data-model, and contracts; covered by an explicit unit case in quickstart Scenario 3. |
| **Budget removal has no existing test coverage** (research Finding 4) — nothing fails if the removal is wrong. | Stage D adds the integration case that exercises the previously-capped path before the branch is considered done. |
| **`index.ts` / `generated.ts` drift** — the hand-maintained duplicate silently disagrees with the API. | `make tygo-check` catches generated-side staleness; the duplicate is edited in the same commit and listed in Complexity Tracking. |
| **Long-running searches now unbounded** — no cap means a pathological search runs until its time limit. | Spec Edge Cases: runs stay bounded by existing execution time limits, and that is reported as a run-duration outcome, not a rate-control one. Unchanged by this work. |

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Hand-maintained duplicate of `HostRetrievalStatusDto` in `packages/shared/src/index.ts` alongside the tygo-generated copy in `generated.ts` (Constitution III) | Pre-existing, not introduced here. The shape changes in this feature, so both copies must be edited together or the dashboard build disagrees with the API. | Properly de-duplicating means re-pointing every consumer of `index.ts` at the generated module — a refactor whose blast radius covers the whole dashboard and is unrelated to daily limits. Bundling it would make this change unreviewable. Recorded so it reads as inherited debt, not as endorsement. |
| FR-009 (crawl-delay-aware pacing) implemented here rather than treated as existing behaviour | The spec assumed it worked; research proved the column is written by a function with no callers and read by nothing. Removing the cap makes pacing the sole protection, so the one signal hosts publish about their own tolerance should not stay unused. | Shipping FR-009 as already-satisfied would be false. Deferring it leaves protection weaker exactly when the cap disappears, working against SC-006. |
