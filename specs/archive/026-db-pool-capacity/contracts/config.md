# Contract: Configuration Keys

Seven new keys. All optional — the shipped defaults are correct for the shipped concurrency defaults. Keys follow the existing convention: `mapstructure` tag equals the env var name, default registered in `internal/config/defaults.go`, documented in `apps/api/.env.example`.

## Keys

| Key | Type | Default | Effect |
|---|---|---|---|
| `DB_MAX_CONNS` | int | `0` (derive) | Maximum database connections. `0` derives from worker concurrency + reserve. |
| `DB_MIN_CONNS` | int | `2` | Connections kept open when idle. Avoids reconnect cost on the first request after quiet. |
| `DB_MAX_CONN_LIFETIME` | duration | `1h` | Maximum age before a connection is retired. |
| `DB_MAX_CONN_IDLE_TIME` | duration | `30m` | Maximum idle time before a connection is retired. Retires connections silently dropped by intermediaries. |
| `DB_ACQUIRE_TIMEOUT` | duration | `5s` | Maximum an interactive request waits for a free connection before failing. |
| `DB_SERVER_MAX_CONNS` | int | `100` | What the database server permits, as declared by the operator. Validated against, never queried. |
| `DB_INTERACTIVE_RESERVE` | int | `8` **(provisional)** | Connections budgeted above total worker concurrency for interactive requests. Not derived from a latency model — SC-001 decides whether 8 is correct. If the measured loaded/idle latency ratio exceeds 1.5, raise this and re-measure. |

Durations use viper's duration parsing, matching existing keys such as `ACTIVITY_STALE_AFTER` and `AI_TASK_TIMEOUT_MATCH`.

## `.env.example` block

```dotenv
# --- Database connection capacity (026-db-pool-capacity) ---
# DB_MAX_CONNS=0 derives capacity as (sum of worker pool sizes) + 2 background
# + DB_INTERACTIVE_RESERVE. With shipped concurrency defaults that is 25.
# Set explicitly only to override; a value below what the workload requires
# fails startup with a message naming the settings in conflict.
DB_MAX_CONNS=0
DB_MIN_CONNS=2
DB_MAX_CONN_LIFETIME=1h
DB_MAX_CONN_IDLE_TIME=30m
# How long an interactive HTTP request waits for a free connection before
# failing with a capacity error instead of hanging indefinitely.
DB_ACQUIRE_TIMEOUT=5s
# What your Postgres max_connections actually is. Declared, not queried —
# used to warn when DB_MAX_CONNS would exceed what the server will grant.
DB_SERVER_MAX_CONNS=100
# Connections budgeted for dashboard/API traffic above total worker concurrency.
DB_INTERACTIVE_RESERVE=8
```

## Validation contract

Every failure names the offending key. Messages are prefixed `config:` to match existing `config.Load` errors.

| Condition | Severity | Message |
|---|---|---|
| `DB_MAX_CONNS < 0` | fail | `config: DB_MAX_CONNS must be >= 0 (0 derives from worker concurrency)` |
| `DB_MIN_CONNS < 0` | fail | `config: DB_MIN_CONNS must be >= 0` |
| `DB_MIN_CONNS > effective DB_MAX_CONNS` | fail | `config: DB_MIN_CONNS (%d) exceeds DB_MAX_CONNS (%d)` |
| `DB_MAX_CONN_LIFETIME <= 0` | fail | `config: DB_MAX_CONN_LIFETIME must be > 0` |
| `DB_MAX_CONN_IDLE_TIME <= 0` | fail | `config: DB_MAX_CONN_IDLE_TIME must be > 0` |
| `DB_ACQUIRE_TIMEOUT <= 0` | fail | `config: DB_ACQUIRE_TIMEOUT must be > 0` |
| `DB_INTERACTIVE_RESERVE < 1` | fail | `config: DB_INTERACTIVE_RESERVE must be >= 1` |
| `DB_SERVER_MAX_CONNS < 1` | fail | `config: DB_SERVER_MAX_CONNS must be >= 1` |
| `DB_MAX_CONNS > 0 && < required` | **fail** | `config: DB_MAX_CONNS=%d is below the %d connections required by worker concurrency (workers=%d background=2 reserve=%d). Raise DB_MAX_CONNS, or lower AI_CONCURRENCY_CLOUD / INGEST_CONCURRENCY / ENRICH_CONCURRENCY.` |
| `effective max > DB_SERVER_MAX_CONNS` | warn | `config: DB_MAX_CONNS=%d exceeds declared DB_SERVER_MAX_CONNS=%d; connections may be refused under load` |
| `DB_MAX_CONN_IDLE_TIME > DB_MAX_CONN_LIFETIME` | warn | `config: DB_MAX_CONN_IDLE_TIME (%s) exceeds DB_MAX_CONN_LIFETIME (%s); idle retirement will never trigger` |

## Startup log line

On successful validation, exactly one info line, so the effective policy is visible without reading configuration:

```
level=INFO msg="db pool configured" max_conns=25 derived=true workers=15 background=2 reserve=8 min_conns=2 lifetime=1h idle=30m acquire_timeout=5s
```

`derived=false` when `DB_MAX_CONNS` was set explicitly.

## Backwards compatibility

Every key is optional with a default. An existing `.env` continues to work unchanged, and the derived default (25) is strictly larger than the previous incidental default (`max(4, NumCPU)`) on any host with fewer than 25 cores. On hosts with more, capacity is reduced to 25 — deliberate, since 25 covers the whole workload with reserve, and the previous value was unrelated to workload. This is the Edge Case the spec raises; it is resolved in favour of the derived value, and `DB_MAX_CONNS` is available for operators who disagree.
