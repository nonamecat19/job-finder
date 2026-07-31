# Contract: `/health/ready` Response Change

**Change type**: additive. No existing field is renamed, removed, or given a new meaning.

**Consumers**: none in this repository. `grep -rn "health" apps/dashboard/src` finds only an unrelated `HealthDot` component rendering per-source status. Verified before designing the change; it is additive regardless.

## Before

```json
{
  "ok": true,
  "checks": {
    "postgres": { "status": "ok", "latency_ms": 3 },
    "redis":    { "status": "ok", "latency_ms": 1 },
    "minio":    { "status": "disabled" }
  }
}
```

## After

```json
{
  "ok": true,
  "checks": {
    "postgres": { "status": "ok", "latency_ms": 3 },
    "redis":    { "status": "ok", "latency_ms": 1 },
    "minio":    { "status": "disabled" }
  },
  "pool": {
    "max_conns": 25,
    "acquired_conns": 4,
    "idle_conns": 6,
    "total_conns": 10,
    "empty_acquire_count": 0,
    "acquire_duration_ms": 0,
    "saturated": false
  }
}
```

## Field semantics

| Field | Kind | Meaning |
|---|---|---|
| `max_conns` | gauge | Configured capacity, after derivation |
| `acquired_conns` | gauge | Connections currently checked out |
| `idle_conns` | gauge | Connections open and free |
| `total_conns` | gauge | `acquired + idle` |
| `empty_acquire_count` | **counter, cumulative since process start** | Acquires that found no free connection and had to wait |
| `acquire_duration_ms` | **counter, cumulative since process start** | Total time spent waiting for connections |
| `saturated` | derived boolean | `acquired_conns >= max_conns` at the instant of the request |

The two counters are cumulative, not rates. A single reading cannot say whether the system is *currently* struggling; two readings can. This is a stated limitation, not an oversight — proper rate treatment belongs with the metrics system (research.md R6).

## Package location

`HealthHandler` lives in `internal/httpapi/health.go` today. **Feature 027 moves it to `internal/health` unconditionally** (027 T039), precisely because this feature's `PoolStatter` would otherwise leave the router package importing `internal/db`. Whichever feature lands second adjusts the wiring in `cmd/server/compose.go`; the design is the same in both orderings.

## Interface

The handler gains a second optional dependency interface, separate from the existing `Pinger`:

```go
// PoolStatter is implemented by the Postgres pool only. Redis and MinIO have
// no equivalent, so this is deliberately not folded into Pinger.
type PoolStatter interface {
    PoolStats() db.PoolStats
}
```

`HealthHandler.Pool` is nil-able. When nil, the `pool` key is omitted entirely rather than emitted with zero values — a zero-valued block is indistinguishable from a genuinely idle pool, and would misreport `max_conns: 0`.

## Readiness verdict

`ok` is **not** affected by pool saturation. A saturated pool is a capacity signal, not an unreadiness signal: a saturated pool is still serving requests, and flipping `ok` to false would take the process out of rotation for a load condition, making it worse. Only a failing `Ping` sets `ok: false`, exactly as today.

## Status code

Unchanged: `200` when `ok` is true, `503` when false. Adding the `pool` block does not change either.
