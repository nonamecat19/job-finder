# Phase 1 Data Model: Explicit Database Connection Capacity

No database schema changes. No migration. The "entities" here are in-process configuration and reporting structures.

---

## 1. `db.PoolConfig`

The explicit pool policy, built in `cmd/server/platform.go` from `config.Config` and passed to `db.Open` via a functional option.

| Field | Type | Source setting | Default | Notes |
|---|---|---|---|---|
| `MaxConns` | `int32` | `DB_MAX_CONNS` | derived (25) | 0/unset ⇒ derive from `CapacityBudget` |
| `MinConns` | `int32` | `DB_MIN_CONNS` | 2 | Retained idle connections; must be ≤ `MaxConns` |
| `MaxConnLifetime` | `time.Duration` | `DB_MAX_CONN_LIFETIME` | `1h` | Retires long-lived connections |
| `MaxConnIdleTime` | `time.Duration` | `DB_MAX_CONN_IDLE_TIME` | `30m` | Retires connections idled out by intermediaries |
| `HealthCheckPeriod` | `time.Duration` | — | `1m` (pgx default) | Not exposed as a setting; no operator need identified |

**Validation** (in `config`, before pgx sees any of it, so errors name the setting):

- `MaxConns < 0` → error `config: DB_MAX_CONNS must be >= 0 (0 means derive)`
- `MaxConns > 0 && MaxConns < budget.Required` → error naming both, see §2
- `MinConns < 0` → error
- `MinConns > MaxConns` → error `config: DB_MIN_CONNS (n) exceeds DB_MAX_CONNS (m)`
- `MaxConnLifetime <= 0` or `MaxConnIdleTime <= 0` → error
- `MaxConnIdleTime > MaxConnLifetime` → warning; idle time can never be reached

**Application**: `pgxpool.ParseConfig(dsn)` → assign the fields above → `pgxpool.NewWithConfig`. Fields not listed retain DSN-supplied or pgx-default values (research.md R2).

---

## 2. `db.CapacityBudget`

Derives required capacity and validates configured capacity against it. Pure arithmetic — no I/O, fully unit-testable.

```
CapacityBudget{
    WorkerSlots       int   // Σ TaskPolicy.PoolSize()
    BackgroundSlots   int   // scheduler + activity sweeper = 2
    InteractiveReserve int  // DB_INTERACTIVE_RESERVE, default 8
    ServerMax         int   // DB_SERVER_MAX_CONNS, default 100
}
```

**Derivation**:

```
Required() = WorkerSlots + BackgroundSlots + InteractiveReserve
```

With shipped defaults: `15 + 2 + 8 = 25`.

**`WorkerSlots` is computed from `queue.PoliciesFromConfig(cfg)`**, summing `TaskPolicy.PoolSize()` — the same values `cmd/server/servers.go` uses to size each `asynq.Server`. Using `PoolSize()` (which is `max(Local, Hosted)`) rather than the live provider class is what satisfies FR-013: a runtime dashboard change cannot push live concurrency past the ceiling already budgeted for.

**`BackgroundSlots = 2`** is a named constant with a comment listing what it covers (ingestion scheduler, activity sweeper). If a third long-lived background goroutine that holds connections is added, this constant must be raised — the constant's comment says so, and `capacity_test.go` asserts the count against the goroutines launched in `runServers`.

**Validation outcomes**:

| Condition | Outcome | Message |
|---|---|---|
| `MaxConns` unset | Derive, use `Required()` | info log stating derived value and breakdown |
| `MaxConns >= Required()` | Pass | — |
| `MaxConns < Required()` | **Fail startup** | names `DB_MAX_CONNS`, the driving concurrency settings, and `Required()` |
| `MaxConns > ServerMax` | **Warn**, continue | names `DB_MAX_CONNS` and `DB_SERVER_MAX_CONNS` |
| `Required() > ServerMax` | **Warn**, continue | the workload cannot fit the declared server limit even in principle |

Failing on under-capacity but only warning on over-capacity is deliberate: under-capacity is a certainty the process can detect entirely on its own, whereas over-capacity is measured against an operator-declared number that may simply be out of date (research.md R4).

---

## 3. `db.PoolStats`

A point-in-time snapshot, produced from `pgxpool.Pool.Stat()`, consumed by the readiness report and the saturation logger.

| Field | JSON | Source | Meaning |
|---|---|---|---|
| `MaxConns` | `max_conns` | `Stat().MaxConns()` | Configured capacity |
| `AcquiredConns` | `acquired_conns` | `Stat().AcquiredConns()` | Currently checked out |
| `IdleConns` | `idle_conns` | `Stat().IdleConns()` | Open and free |
| `TotalConns` | `total_conns` | `Stat().TotalConns()` | Open, acquired + idle |
| `EmptyAcquireCount` | `empty_acquire_count` | `Stat().EmptyAcquireCount()` | Cumulative: acquires that had to wait |
| `AcquireDurationMs` | `acquire_duration_ms` | `Stat().AcquireDuration()` | Cumulative wait time |

`EmptyAcquireCount` and `AcquireDuration` are **cumulative since process start**, not gauges. They are reported raw; interpreting them as rates is the observability feature's job (research.md R6).

**Derived field**:

```
Saturated = AcquiredConns >= MaxConns
```

---

## 4. Saturation record (log-only, not persisted)

The saturation logger samples pool state on a ticker (`30s`, aligned with the existing `ACTIVITY_HEARTBEAT_INTERVAL` convention) and emits a `slog.Warn` only when the pool has been continuously saturated across a threshold number of consecutive samples (default 4 ⇒ ~2 minutes).

Fields: `max_conns`, `acquired_conns`, `empty_acquire_count_delta`, `consecutive_saturated_samples`.

Emitting only on *sustained* saturation is what distinguishes capacity exhaustion from an ordinary burst, which is FR-009's actual requirement. A single saturated sample is normal and must not log.

---

## 5. Accepted scope limits

- **Acquisition bounding covers interactive requests only.** Background workers remain bounded by their existing `TaskPolicy.MaxDuration` deadlines rather than a dedicated acquisition timeout, because there is no single choke point to wrap without shimming sqlc's generated `DBTX` interface (research.md R5). FR-008a is satisfied for the path SC-001 and SC-002 measure.
- **`EmptyAcquireCount` is cumulative, not a gauge.** FR-008's full intent needs a metrics system; deferred (plan.md Complexity Tracking).
- **No per-caller attribution.** The statistics say the pool is exhausted, not who exhausted it. Attribution needs tracing, which is the observability feature.
