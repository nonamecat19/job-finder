---
title: Request lifecycle
sidebar_position: 3
description: An HTTP request from socket to JSON — middleware, mounts, handler, service, repository, and error mapping.
---

# Request lifecycle

## The middleware chain

`NewRouter` (`internal/httpapi/router.go:13-41`) builds the chain once, for all routes:

```mermaid
flowchart LR
    REQ["Request"] --> RID["middleware.RequestID"]
    RID --> IP["middleware.RealIP"]
    IP --> LOG["requestLogger"]
    LOG --> REC["middleware.Recoverer"]
    REC --> CORS["cors.Handler"]
    CORS --> MUX{"route match?"}
    MUX -->|"/api/... or /api/v1/..."| H["handler"]
    MUX -->|no| NF["404 JSON: not found: path"]
    H --> RES["JSON response"]
```

Notable choices:

- **Dual mount.** The same `mountAll` closure is registered under `/api` and `/api/v1`
  (`router.go:38-39`). Handlers therefore declare unversioned paths; adding a version
  prefix inside a `Mount` produces `/api/v1/v1/...`.
- **`/health` is inside `mountAll`** (`router.go:28`), so it exists at both prefixes, and
  it is a liveness probe only: `{ok, uptime}`. Readiness is a separate handler at
  `/health/ready` (`internal/httpapi/health.go:34`).
- **CORS is permissive with credentials off.** Appropriate for a self-hosted single-user
  app served from a different dev-server origin.
- **`Recoverer` before the handlers** means a panic becomes a 500 rather than killing the
  process.

## A read request, end to end

```mermaid
sequenceDiagram
    autonumber
    participant B as Browser
    participant R as chi router
    participant H as JobsHandler
    participant S as jobs.Service
    participant Q as sqlcgen.Queries
    participant P as Postgres
    B->>R: GET /api/jobs?status=found
    R->>R: RequestID, RealIP, log, recover, CORS
    R->>H: h.list(w, r)
    H->>H: parse and validate query params
    H->>S: List(ctx, filter)
    S->>Q: ListJobs(ctx, params)
    Q->>P: SELECT ...
    P-->>Q: rows
    Q-->>S: []sqlcgen.Job
    S-->>H: domain results
    H->>H: map to dto
    H-->>B: 200 JSON
```

The mapping step is deliberate: generated row structs never appear on the wire. See
[Domain modeling](/principles/domain-modeling).

## A write request that starts async work

```mermaid
sequenceDiagram
    autonumber
    participant B as Browser
    participant H as JobsHandler
    participant A as activity.Recorder
    participant C as asynq.Client
    participant W as generate worker
    B->>H: POST /api/jobs/{id}/generate
    H->>H: validate job exists
    H->>A: create ActivityRun (queued)
    H->>C: Enqueue(generate payload)
    H-->>B: 202 with activity id
    C-->>W: deliver
    W->>A: running + heartbeat
    W->>W: build documents, render PDF
    W->>A: succeeded
    B->>H: GET /api/activity (poll)
```

The HTTP request returns as soon as the task is durable in Redis. Progress is observed
through `/api/activity`, never by holding the connection open.

## Error mapping

```mermaid
flowchart TD
    S["service returns error"] --> W["writeAppError(w, err)"]
    W --> AS{"errors.As *apperr.Error"}
    AS -->|yes| MAP["apperr.HTTPStatusCode(kind)"]
    AS -->|no| G["500 + generic message"]
    MAP --> BODY["ErrorResponse: message + code"]
    G --> BODY2["ErrorResponse: internal"]
```

Response envelope (`internal/httpapi/helpers.go:12-16`):

```json
{ "message": "job 8f3c… not found", "code": "not_found" }
```

## Handler conventions

| Concern | Convention |
| --- | --- |
| Registration | `func (h *XHandler) Mount(r chi.Router)`, passed to `NewRouter` in `servers.go` |
| Path params | `chi.URLParam(r, "id")`, validated before use |
| Body decoding | `decodeJSON(r, &dst)` (`helpers.go:51-54`) |
| Success | `writeJSON(w, status, payload)` |
| Failure | `writeAppError(w, err)` — never a hand-picked status |
| Multipart | detected by `Content-Type` prefix, e.g. `profiles.go:133`, `referral.go:59` |

## Timeouts and shutdown

`http.Server` sets `ReadHeaderTimeout: 10s` (`cmd/server/servers.go:71`). On SIGINT or
SIGTERM the context from `signal.NotifyContext` (`main.go:31`) cancels; workers shut down
first, then the HTTP server drains with a 10-second timeout
(`servers.go:108-114`).

```mermaid
stateDiagram-v2
    [*] --> Serving
    Serving --> Draining: SIGINT or SIGTERM
    Draining --> WorkersStopped: asynq Shutdown for each worker
    WorkersStopped --> HTTPDrained: Shutdown with 10s timeout
    HTTPDrained --> Closed: deferred DB, scraping, asynq closes
    Closed --> [*]
```
