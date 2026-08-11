---
title: Ingestion overview
sidebar_position: 1
description: The end-to-end discovery pipeline and the principles that govern it.
---

# Ingestion overview

Ingestion is the path from "a job exists on some board" to "a `Job` row exists here,
scored and ready to read".

## The pipeline

```mermaid
flowchart TD
    T1["Scheduler tick every 5 minutes"] --> DUE{"SavedSearch due by cron?"}
    DUE -->|yes| CLAIM["ClaimSavedSearchRun — compare-and-swap on lastRunAt"]
    CLAIM -->|won| ENQ["enqueue ingest per enabled source"]
    CLAIM -->|lost| SKIP["another runner already claimed the slot"]
    MAN["Manual: POST /sources/{key}/run or /searches/{id}/run"] --> ENQ
    SUB["Subscription cron"] --> ENQ
    ENQ --> W["ingest worker"]
    W --> REG["Registry resolves the adapter by key"]
    REG --> CFG["decrypt JobSource.config, merge over env defaults"]
    CFG --> SEARCH["Adapter.Search via the retrieval ladder"]
    SEARCH --> NORM["[]dto.NormalizedJob"]
    NORM --> DED["dedupe"]
    DED --> INS["insert new Job rows"]
    INS --> SR["SourceRun: found / new / ok / error"]
    INS --> FAN["enqueue match, enrich, ghost:score, salary:infer"]
```

## Principles

### One retrieval path for every adapter

`ports.Retriever` is the single shared HTTP retrieval interface, implemented by the
library's `retrieval.Engine` and configured here by `internal/retrieval`. The port package
is explicit about why (`ports/doc.go`): every other package depends on the port rather
than on a concrete implementation, so no source implements its own request strategy or
challenge handling.

An adapter therefore cannot accidentally bypass pacing, identity, cookies, or challenge
handling — those are properties of the transport, not of adapter discipline.

### Adding a source is one file plus one registry line

Adding a job site is one adapter package implementing `ports.JobSource` in the job-scraper
library (`adapters/<key>/`), plus one entry in the registry constructor list — the
`adapter.NewRegistry(...)` call in `cmd/server/compose.go:186-201`.

### Capabilities are optional interfaces, not flags

```mermaid
classDiagram
    class Adapter {
        <<interface>>
        +Key() string
        +Kind() SourceKind
        +Search(ctx, query, config) NormalizedJob[]
        +HealthCheck(ctx, config) bool
    }
    class DetailNeeder {
        <<interface>>
        +NeedsDetail() bool
    }
    class Credentialed {
        <<interface>>
        +UsesUserAccount() bool
    }
    Adapter <|.. DjinniAdapter
    DetailNeeder <|.. DjinniAdapter
    Credentialed <|.. JobLeadsAdapter
    Adapter <|.. JobLeadsAdapter
```

An adapter whose `Search` already returns complete rows implements nothing extra;
`jobsources.NeedsDetail(a)` and `IsCredentialed(a)` type-assert and default to `false`
(`adapter.go:44-58`). New capabilities are added without touching twenty adapters.

### One source's failure is not the run's failure

Each source is its own `ingest` task with its own `SourceRun` row. A 503 from one board
marks that run `ok=false` with an `error`, and the others complete normally.

### Retries are for transient failures only

`IngestMaxRetry = 2` — three deliveries. Errors a retry cannot fix are wrapped in
`asynq.SkipRetry` by the handler (`internal/queue/queue.go:39-49`). The comment records
why the value changed from zero: scraping is the least reliable step and runs are hours
apart, so one transient failure used to cost a source its entire cron window.

### Credentialed sources never escalate

A request with `UsesUserAccount` set stops at the rung it is on rather than escalating
(`retrieval/engine.go:135-141`). The browser and FlareSolverr rungs cannot carry the session
cookie and would land on a login page — and driving a challenge solver through someone's
account is not something to do quietly.

### Normalize at the boundary, keep the original

Adapters return `dto.NormalizedJob`. The provider payload is stored in `Job.raw` so a
parser fix can be replayed without re-scraping.

## Where things live

| Concern | Package |
| --- | --- |
| Adapter contract, registry, ~20 adapters | `internal/jobsources` |
| Scheduling, runs, dedupe, reconciliation | `internal/ingestion` |
| Fetch ladder, identity, per-host state | `internal/retrieval` |
| Headless browsing | `internal/scraping` |
| Per-host pacing | `internal/ratelimit` |
| Ghost-posting detection | `internal/ghostjob` |
| ATS board roster | `internal/jobsources/roster` |

## Observing a run

```mermaid
sequenceDiagram
    participant U as Dashboard Sources page
    participant API as /api/sources/{key}/run
    participant Q as ingest queue
    participant W as worker
    participant AR as ActivityRun
    participant SR as SourceRun
    U->>API: run this source
    API->>AR: create run (queued)
    API->>Q: enqueue IngestPayload
    Q->>W: deliver
    W->>AR: running + heartbeat
    W->>SR: startedAt
    W->>SR: finishedAt, ok, found, new, error
    W->>AR: succeeded or failed
    U->>API: GET /searches/runs/recent
```
