---
title: Observability
sidebar_position: 4
description: Liveness and readiness endpoints, activity runs, queue visibility, and logging conventions.
---

# Observability

Four windows into a running system: health endpoints, `ActivityRun`, the queue view, and
structured logs.

```mermaid
flowchart TB
    A["GET /api/health — liveness"] --> P["is the process up?"]
    B["GET /api/health/ready — readiness"] --> D["are dependencies reachable?"]
    C["GET /api/activity — runs"] --> W["what is each task doing?"]
    E["GET /api/activity/queues + asynqmon"] --> Q["what is queued and where will it run?"]
    F["slog output"] --> L["what happened and why?"]
```

## Liveness

```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]any{
        "ok":     true,
        "uptime": time.Since(startTime).Seconds(),
    })
}
```

Mounted inside `mountAll` (`router.go:28`), so it exists at `/api/health` and
`/api/v1/health`. It touches nothing — it answers "is the process serving HTTP".

## Readiness

Spec 008. `HealthHandler` (`internal/httpapi/health.go`) checks each dependency through a
structural port:

```go
type Pinger interface {
    Ping(ctx context.Context) error
}

type HealthHandler struct {
    Postgres Pinger
    Redis    Pinger
    Minio    Pinger // nil if MinIO is not configured
}
```

`*pgxpool.Pool`, `redis.UniversalClient` and `*minio.Client` (via a small adapter) all
satisfy it with their existing methods.

```mermaid
flowchart TD
    R["GET /api/health/ready"] --> PG{"postgres"}
    R --> RD{"redis"}
    R --> MN{"minio"}
    PG -->|nil| PD["status: disabled"]
    PG -->|"Ping within 2s"| POK["status: ok, latency_ms"]
    PG -->|error or timeout| PE["status: error, error, latency_ms → ok=false"]
    RD --> ROK["same"]
    MN -->|nil| MD["status: disabled — not a failure"]
    POK --> AGG["ok=true → 200"]
    PE --> AGG2["ok=false → 503"]
```

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

Three design points:

1. **Each ping has a 2-second bound** (`readinessTimeout`), *"so a hung dependency cannot
   hang the whole /health/ready response"* — the classic readiness failure mode.
2. **Latency is reported per dependency**, so a slow-but-alive Postgres is visible before
   it becomes an outage.
3. **`nil` means `disabled`, not unhealthy.** Unconfigured MinIO reports `disabled` and
   readiness stays green, mirroring the storage package's convention.

Status codes: `200` when all configured checks pass, `503` otherwise.

## Activity runs

Every async operation writes an `ActivityRun` — states, steps, heartbeats, terminal
reasons, and the sweeper that closes out vanished workers.
See [activity tracking](/async/activity-tracking).

| Endpoint | Answers |
| --- | --- |
| `GET /api/activity` | what ran, what is running, what failed and why |
| `GET /api/activity/queues` | backlog per queue and the provider class each will use |
| `GET /api/searches/runs/recent` | per-source scrape outcomes (`SourceRun`) |
| `GET /api/hosts/{host}/retrieval-status` | current rung, blocks, cooling-off, crawl delay |

## Queue visibility

asynqmon on :8090 in development ([monitoring](/async/monitoring)). Redis-level truth —
payloads, retry counts, archived tasks — complementing the business-level view in
`ActivityRun`.

## Logging

Structured `log/slog`, with the default logger held on `Platform.Logger`.

| Level | Used for |
| --- | --- |
| `Error` | 5xx responses, worker crashes, sweeper query failures |
| `Warn` | degraded-but-working: disabled optional source, unreadable host state, browser rung unavailable |
| `Info` | lifecycle: listening, shutting down, swept N runs, FlareSolverr configured |

The `Warn` level carries real meaning here — it is the *"this feature degraded rather than
failed"* channel:

| Message | Meaning |
| --- | --- |
| `retrieval: browser rung unavailable, will skip` | the ladder is direct-only |
| `retrieval: get state failed, proceeding with direct` | per-host state unreadable, fetch continues |
| `salary: LEVELS_FYI_CSV not set — levels.fyi source disabled` | one salary source is off |
| `scheduler: bad cron expression` | one saved search will never fire |
| `notifier: job not found` | a notification was skipped, harmlessly |

`writeError` and `writeAppError` log **only** at 5xx (`helpers.go:29-49`): a 404 is a normal
outcome, not an incident.

## No metrics endpoint

There is no Prometheus endpoint. The observable surface is the health endpoints, the
activity API, asynqmon and the logs. For a single-user self-hosted deployment that is the
proportionate answer; if you add metrics, the natural counters already exist —
`ActivityRun` states, `SourceRun` found/new, and queue depths from the asynq inspector.

## A monitoring loop

```mermaid
sequenceDiagram
    participant W as Watchdog
    participant API as /api/health/ready
    participant ACT as /api/activity
    participant AM as asynqmon
    loop every minute
        W->>API: readiness
        alt 503
            W->>W: alert — read `checks` for the failing dependency
        end
    end
    loop when something looks wrong
        W->>ACT: recent runs — any interrupted / failed / timed_out?
        W->>AM: queue depth and retry counts
    end
```

## What each symptom points at

| Symptom | Look at |
| --- | --- |
| Dashboard shows nothing | `/api/health` — is the process up? |
| 503 from readiness | the `checks` object names the dependency |
| Jobs stop appearing | `SourceRun` via `/api/searches/runs/recent`, then host retrieval status |
| Matches stop appearing | `/api/activity` for the `match` queue; provider errors |
| Runs stuck `running` | heartbeat and sweeper settings; asynqmon for the real task state |
| Everything slow | queue depth versus concurrency, and provider latency in the logs |
