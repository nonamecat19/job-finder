# Phase 0 Research: Explicit Database Connection Capacity

All Technical Context unknowns resolved. Findings verified against this repository and against the vendored dependency source, not from memory.

---

## R1 — What is the pool actually sized to today?

**Decision**: `max(4, runtime.NumCPU())`. The problem statement's claim is confirmed, not assumed.

**Evidence**: `internal/db/db.go:32` calls `pgxpool.New(ctx, databaseURL)` with no configuration. In the vendored dependency:

```
$ grep -n "defaultMaxConns\|NumCPU" ~/go/pkg/mod/github.com/jackc/pgx/v5@v5.10.0/pgxpool/pool.go
19:	defaultMaxConns          = int32(4)
175:	// MaxConns is the maximum size of the pool. The default is the greater of 4 or runtime.NumCPU().
381:		config.MaxConns = defaultMaxConns
382:		if numCPU := int32(runtime.NumCPU()); numCPU > config.MaxConns {
383:			config.MaxConns = numCPU
```

**Consequence**: on a 4-core self-hosted box the pool is 4, against 15 default worker slots plus HTTP. On a 16-core box it happens to be adequate — which is why this has not been catastrophic and why it must not be "fixed" by an explicit value *smaller* than the incidental default on large hosts (spec Edge Cases).

---

## R2 — How should the pool be configured, given `pgxpool.New` takes only a URL?

**Decision**: `pgxpool.ParseConfig(url)` → mutate the returned `*pgxpool.Config` → `pgxpool.NewWithConfig(ctx, cfg)`.

**Rationale**: this is pgx's documented mechanism and the only way to set `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime` and `HealthCheckPeriod` programmatically. The alternative — appending `pool_max_conns=25` and friends to the DSN — was rejected: it makes capacity a property of a connection string that is also a secret, splits configuration across two mechanisms, and produces an opaque parse error rather than a message naming the setting (FR-005).

**Consequence**: `db.Open` must not lose the DSN's own parameters. `ParseConfig` preserves them; only fields explicitly set afterwards are overridden. Fields left alone keep DSN-supplied or pgx-default values.

---

## R3 — What should the default capacity be, and how is it derived?

**Decision**: when unset, capacity is derived as `sum(TaskPolicy.PoolSize() for all policies) + fixed non-worker allowance + interactive reserve`. With shipped defaults this is `15 + 2 + 8 = 25`.

**Evidence**: worker pool sizes come from `queue.PoliciesFromConfig(cfg)` and `TaskPolicy.PoolSize()`, already used by `cmd/server/servers.go` to size each `asynq.Server`. At `internal/config/defaults.go` values:

| Queue | Source setting | Default | `PoolSize()` |
|---|---|---|---|
| ingest | `INGEST_CONCURRENCY` | 2 | 2 |
| match | `AI_CONCURRENCY_LOCAL` / `_CLOUD` | 1 / 3 | 3 |
| generate | `AI_CONCURRENCY_LOCAL` / `_CLOUD` | 1 / 3 | 3 |
| enrich | `ENRICH_CONCURRENCY` | 1 | 1 |
| salary | `AI_CONCURRENCY_LOCAL` / `_CLOUD` | 1 / 3 | 3 |
| ghost | `AI_CONCURRENCY_LOCAL` / `_CLOUD` | 1 / 3 | 3 |
| | | **total** | **15** |

Non-worker background holders: the ingestion scheduler (`worker.Scheduler.Run`) and the activity sweeper (`p.Sweeper.Run`), both launched as goroutines in `runServers` — allowance 2.

**Rationale for deriving rather than hardcoding**: a fixed default goes stale the instant any concurrency default changes, which is the exact failure mode being fixed. Deriving means the two settings cannot drift apart.

**Rationale for using `PoolSize()` and not live concurrency**: `PoolSize()` returns `max(Local, Hosted)` precisely because `asynq.Config.Concurrency` is fixed at server construction while the admission gate varies the effective limit at runtime. Sizing against the live value would under-provision the moment a dashboard settings change flips a task to a hosted provider — which is FR-013, satisfied for free by using the ceiling.

**Alternative rejected**: deriving from `runtime.NumCPU()`. That is what pgx already does and is unrelated to how many connections this workload holds.

---

## R4 — How is the database server's own limit accounted for?

**Decision**: a declared `DB_SERVER_MAX_CONNS` setting, default 100, validated against at startup. Not interrogated from the server.

**Evidence**: PostgreSQL's `max_connections` default is 100, and the project's `docker-compose.yml` runs `pgvector/pgvector:pg16` with no override, so 100 is the real value in the shipped stack.

**Rationale**: interrogating requires connecting first, so a misconfiguration is discovered *after* the pool is already open, and the check would issue a query on every boot to learn something the operator configured. Declaring it keeps validation entirely in the config layer where every other startup check lives.

**Consequence**: the check is advisory about a value the operator may state wrongly. It is still worth having — it catches the common case of raising `DB_MAX_CONNS` past a server that was never tuned. The validation warns rather than fails when capacity exceeds the declared server limit but the process is otherwise viable, and fails only when capacity is below what the workload requires.

---

## R5 — What are the semantics of a bounded acquisition wait?

**Decision**: a `DB_ACQUIRE_TIMEOUT` (default 5s) applied by wrapping the acquire path, surfacing a distinguishable error on expiry.

**Evidence**: `pgxpool` has no `AcquireTimeout` field — `Acquire(ctx)` blocks until a connection is free or `ctx` is done. Bounding therefore means deriving a `context.WithTimeout` at the acquire site, not setting a pool option.

**Rationale**: unbounded waiting is what makes the current symptom undiagnosable — a starved HTTP request is indistinguishable from a slow query, which is the spec's central complaint. A bounded wait converts it into a fast, attributable error.

**Consequence and limit**: `*sqlcgen.Queries` is constructed over the pool and calls `Acquire` internally per query, so there is no single choke point to wrap without touching generated code. Two options were considered:

- **Chosen**: set the timeout on the *inbound* boundaries instead — HTTP requests already carry a request context, and worker tasks carry a task context with `MaxDuration` from `TaskPolicy`. Adding a shorter, explicit acquisition deadline at the HTTP middleware layer bounds the interactive case, which is the case that matters for SC-001/SC-002. Background workers keep their existing longer task deadlines.
- Rejected: a custom `DBTX` wrapper interposed between `pgxpool.Pool` and `sqlcgen.New`. It would cover every path uniformly, but `DBTX` has four methods with pgx-specific signatures, and wrapping them means owning a shim that must track sqlc's generated interface across regenerations. Disproportionate for the benefit.

**This narrows FR-008a**: the bounded wait is guaranteed for interactive requests. Background workers remain bounded by their existing task deadlines rather than by a dedicated acquisition timeout. Recorded as an accepted scope limit in data-model.md.

---

## R6 — Where do pool statistics surface, given no metrics system exists?

**Decision**: extend the existing `/health/ready` report with a `pool` block, plus a sampled saturation log line. No new endpoint, no metrics library.

**Evidence**: `internal/httpapi/health.go` already serves `/health/ready` with a per-dependency `checks` map, and `pgxpool.Pool.Stat()` returns `*pgxpool.Stat` carrying `MaxConns()`, `AcquiredConns()`, `IdleConns()`, `TotalConns()`, `EmptyAcquireCount()` and `AcquireDuration()` — every quantity FR-007 and FR-008 name, available with no instrumentation.

**Confirmed safe**: no dashboard code reads `/health/ready` (`grep -rn "health" apps/dashboard/src` returns only an unrelated `HealthDot` source-status component), so adding fields breaks no consumer. The change is additive regardless.

**Design note**: the readiness handler's `Pinger` interface is deliberately minimal and is satisfied by Redis and MinIO too. Pool statistics need a *separate* optional interface (`PoolStatter`), left nil for dependencies that have no pool — widening `Pinger` would force meaningless implementations on the other two.

**Deferral**: FR-008 asks for waiting-caller *metrics*. `EmptyAcquireCount` and `AcquireDuration` are cumulative counters, correct for a scrape-based system and awkward as a point-in-time report. They are exposed in the readiness block as raw cumulative values, and proper metric treatment is deferred to the observability feature — recorded in plan.md Complexity Tracking.

---

## Summary of decisions

| ID | Decision |
|---|---|
| R1 | Confirmed current sizing is `max(4, NumCPU)` — verified in dependency source |
| R2 | `ParseConfig` → mutate → `NewWithConfig`; not DSN parameters |
| R3 | Capacity derived from `TaskPolicy.PoolSize()` sum + 2 + reserve 8 = 25 by default |
| R4 | Server limit declared via config (default 100), not interrogated |
| R5 | Acquisition bounded at the HTTP boundary; workers keep task deadlines |
| R6 | Statistics via existing `/health/ready` + sampled log; no metrics library |
