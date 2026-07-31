# Implementation Plan: Explicit Database Connection Capacity

**Branch**: `026-db-pool-capacity` | **Date**: 2026-07-30 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/026-db-pool-capacity/spec.md`

## Summary

`db.Open` calls `pgxpool.New(ctx, databaseURL)` with no pool configuration, so `MaxConns` takes pgx's default of `max(4, runtime.NumCPU())`. Six asynq worker servers totalling 15 slots at default config, plus the scheduler, the activity sweeper and every HTTP request, all draw on that pool. On a 4-core host the pool is 4.

Four changes, all in `apps/api`, no API or dashboard contract change:

1. **Explicit pool config** — `db.Open` gains variadic options and goes through `pgxpool.ParseConfig` so `MaxConns`, `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime` and `HealthCheckPeriod` are set from `config.Config` rather than defaulted.
2. **Derived default + startup validation** — a new `db.CapacityBudget` computes required capacity from `queue.PoliciesFromConfig` (the same source `buildServers` already uses to size worker pools) plus a fixed interactive reserve, and fails startup when the configured capacity cannot cover it.
3. **Bounded acquisition** — a per-acquire timeout so exhaustion surfaces as an attributable error instead of an unbounded wait.
4. **Saturation visibility** — pool statistics added to the existing `/health/ready` report, plus a sampled saturation log line.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api` only)

**Primary Dependencies**: existing — `jackc/pgx/v5` v5.10.0 (`pgxpool`), `spf13/viper` v1.21.0, `hibiken/asynq` v0.26.0, `go-chi/chi/v5`. **No new dependencies.**

**Storage**: PostgreSQL 16 (`pgvector/pgvector:pg16`). No schema change, no migration.

**Testing**: `go test ./...` for config/budget unit tests; `go test -tags integration ./...` for pool behaviour against real Postgres, following the existing `internal/dbtest` helper convention.

**Target Platform**: Linux; Docker Compose dev and prod stacks.

**Project Type**: Backend service change confined to `apps/api/internal/{config,db,queue,httpapi}` and `cmd/server`.

**Performance Goals**: interactive requests complete within 150% of idle-system latency while all worker pools are saturated (SC-001); zero connection-unavailable failures under sustained full background load (SC-002).

**Constraints**:

- `pgxpool.Config.MaxConns` is `int32` and must be ≥ 1; pgx rejects 0 at `ParseConfig` validation, so the config layer must reject it first with a better message.
- `MinConns` must not exceed `MaxConns` — pgx does not validate this pairing, it just behaves oddly, so the budget check must.
- Worker pool sizes come from `TaskPolicy.PoolSize()`, which returns `max(LocalConcurrency, HostedConcurrency)` because `asynq.Config.Concurrency` is fixed at server construction. The budget must use `PoolSize()`, **not** the currently-resolved provider class — this is exactly FR-013: a dashboard flip from local to hosted raises live concurrency without a restart, and `PoolSize()` already accounts for the ceiling.
- Default worker total today: ingest 2 + match 3 + generate 3 + enrich 1 + salary 3 + ghost 3 = **15**. Plus scheduler (1) and activity sweeper (1) = 17. Interactive reserve 8 → derived default **25**.
- **`db.Open` has 15 call sites, 13 of them tests** (`grep -rn "db.Open(" apps/api`). A required third parameter would force 13 mechanical test edits into a change that is otherwise 6 files, burying the reviewable part. `db.Open(ctx, url, opts ...Option)` is used instead: existing call sites compile unchanged and keep pgx's defaults, and only `cmd/server/platform.go` passes `db.WithPoolConfig(...)`. Tests that need to assert pool behaviour opt in explicitly.
- The readiness handler's `Pinger` interface is deliberately narrow (`Ping(ctx) error`). Pool statistics need a second, separately optional interface rather than widening `Pinger`, so Redis and MinIO are unaffected.

**Scale/Scope**: single Postgres instance, single API process, one user. ~8 files touched, no migration, no generated-code regeneration.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment |
|---|---|
| I. No Auto-Apply, Ever | **N/A** — no code path touching applications or employers. |
| II. Grounded Generation | **N/A** — no LLM-generated content. |
| III. Typed Contracts Across Service Boundaries | **Respected.** The readiness report gains fields; its DTO lives in `internal/httpapi/health.go` and is not part of `packages/shared`. Verified: no dashboard code reads `/health/ready`. No tygo/sqlc regeneration triggered. |
| IV. Test Discipline Per Language, Enforced at the Boundary | **Respected.** Budget arithmetic and config validation get Go unit tests; pool saturation behaviour gets an integration test against real Postgres per the principle's "real infrastructure, not mocks" requirement. No dashboard change, so no Vitest work. |
| V. Local-First, Self-Hosted by Default | **Strengthened.** Removes a failure mode where a modest self-hosted host silently under-provisions itself. No external service introduced. |

**Re-check after Phase 1**: no violations introduced by the design below. One deliberate deferral (FR-008) is recorded in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/026-db-pool-capacity/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── config.md        # New configuration keys, defaults, validation rules
│   └── readiness.md     # /health/ready response shape change
├── checklists/
│   └── requirements.md  # From /speckit-specify
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
apps/api/
├── cmd/
│   └── server/
│       ├── platform.go            # buildPlatform: build budget, db.Open(..., db.WithPoolConfig(..))
│       └── compose.go             # wire pool stats into HealthHandler
├── internal/
│   ├── config/
│   │   ├── config.go              # + DBMaxConns, DBMinConns, DBMaxConnLifetime,
│   │   │                          #   DBMaxConnIdleTime, DBAcquireTimeout,
│   │   │                          #   DBServerMaxConns, DBInteractiveReserve
│   │   ├── defaults.go            # + defaults for the above
│   │   └── config_test.go         # + validation tests
│   ├── db/
│   │   ├── db.go                  # Open(ctx, url, opts ...Option); Option; Stats()
│   │   ├── capacity.go            # NEW: CapacityBudget, Require(), Validate()
│   │   ├── capacity_test.go       # NEW: budget arithmetic + validation
│   │   ├── pool_integration_test.go # NEW: //go:build integration
│   │   └── saturation.go          # NEW: sampled saturation logger
│   └── httpapi/
│       ├── health.go              # + PoolStatter, pool block in readiness report
│       └── health_test.go         # + pool-stats assertions
└── .env.example                   # + documented keys
```

**Structure Decision**: No new top-level structure. `capacity.go` and `saturation.go` join the existing `internal/db` package because they are pool concerns and have no consumer outside it; putting them in `internal/platform` would invert the dependency (`platform` does not import `db` today).

## Phase 0: Research

See [research.md](./research.md). Six questions resolved: pgx default sizing (R1), configuration mechanism (R2), the derived-default formula (R3), server-limit validation (R4), acquisition-timeout semantics (R5), and statistics surface (R6).

## Phase 1: Design

- [data-model.md](./data-model.md) — `PoolConfig`, `CapacityBudget`, `PoolStats`, and the derivation rules.
- [contracts/config.md](./contracts/config.md) — seven new keys with types, defaults, validation, and failure messages.
- [contracts/readiness.md](./contracts/readiness.md) — the additive `/health/ready` change.
- [quickstart.md](./quickstart.md) — how to verify each acceptance scenario by hand.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| FR-008 (waiting-caller metrics) deferred out of scope | The project has no metrics surface at all — no Prometheus registry, no expvar, no OTel. Delivering FR-008 means first delivering a metrics subsystem, which is a separate feature (architecture-review item A) an order of magnitude larger than this one. | Building a minimal metrics endpoint here was rejected: it would be a second, throwaway surface that the real observability feature would immediately replace, and it would make this feature unreviewable as a focused change. FR-007 (readiness statistics) and FR-009 (saturation log) are delivered instead and cover the diagnostic need. |
| Seven new configuration keys | Each maps to a distinct pool property the spec requires explicitly (FR-001, FR-002, FR-004, FR-008a). | Collapsing them into one "pool profile" enum was rejected: the operator needs to raise capacity independently of lifetime when the database is behind a connection-idle-timeout proxy, and an enum forces an unrelated change. |
