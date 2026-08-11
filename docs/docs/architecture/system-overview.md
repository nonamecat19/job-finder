---
title: System overview
sidebar_position: 1
description: Containers, boundaries, and the runtime shape of the whole platform.
---

# System overview

## Context

```mermaid
C4Context
    title Job Finder — system context
    Person(user, "Job seeker", "Runs this on their own machine")
    System(jf, "Job Finder", "Discovery, scoring, document generation, tracking")
    System_Ext(boards, "Job boards and ATS APIs", "Djinni, Indeed, Greenhouse, Lever, ...")
    System_Ext(ollama, "Ollama", "Local or hosted inference")
    System_Ext(cerebras, "Cerebras", "Optional hosted chat provider")
    System_Ext(flare, "FlareSolverr", "Optional challenge solver")
    Rel(user, jf, "Uses the dashboard")
    Rel(jf, boards, "Scrapes and calls APIs")
    Rel(jf, ollama, "Chat and embeddings")
    Rel(jf, cerebras, "Chat, when configured")
    Rel(jf, flare, "Escalated retrieval")
```

## Containers

```mermaid
flowchart TB
    U(["User"])
    subgraph Browser
        DASH["Dashboard<br/>React 19, Vite 6, TanStack Query"]
    end
    subgraph Process["apps/api — one Go binary"]
        HTTP["HTTP API<br/>chi router, /api and /api/v1"]
        W1["ingest worker"]
        W2["match worker"]
        W3["generate worker"]
        W4["enrich worker"]
        W5["salary worker"]
        W6["ghost worker"]
        SCHED["Ingestion scheduler"]
        SWEEP["Activity sweeper"]
    end
    subgraph Data
        PG[("Postgres 16 + pgvector")]
        RD[("Redis — asynq")]
        FS[("Documents: MinIO or disk")]
    end
    SIDE["JobSpy service (external, JOBSPY_URL)"]
    U --> DASH --> HTTP
    HTTP --> PG
    HTTP -->|enqueue| RD
    SCHED -->|enqueue| RD
    RD --> W1 & W2 & W3 & W4 & W5 & W6
    W1 --> SIDE
    W1 & W2 & W3 & W4 & W5 & W6 --> PG
    W3 --> FS
    SWEEP --> PG
```

Everything inside `Process` is a goroutine started by `runServers`
(`cmd/server/servers.go:86-114`). There is no inter-service RPC to configure, secure or
debug.

## Why one binary

| Force | Consequence |
| --- | --- |
| Single-user, self-hosted | no multi-tenant isolation, no horizontal scaling requirement |
| Workers need the same DB and config as the API | shared process avoids duplicating wiring |
| Operators are the users | one `make run-backend` beats a compose topology |
| But: task types have very different concurrency | six **separate** asynq servers, one per type |

The last row is the interesting one. Rather than one worker pool with weighted queues,
each task type gets its own `asynq.Server` so `Concurrency` is a hard per-type ceiling
(`internal/queue/queue.go:22-37`, `cmd/server/servers.go:29-44`).

## Runtime boundaries

```mermaid
flowchart LR
    subgraph Inbound
        REST["REST /api/*"]
    end
    subgraph Core["Domain services"]
        MATCH["matching"]
        GEN["generation"]
        ING["ingestion"]
        MISC["salary, keyword, coach, intel, referral, outreach..."]
    end
    subgraph Outbound["Adapter edges"]
        SQL["sqlc repositories"]
        LLMR["llm.Router"]
        SRC["jobsources adapters"]
        RET["retrieval ladder"]
        STORE["storage"]
        Q["asynq client"]
    end
    REST --> Core
    Core --> SQL & LLMR & SRC & RET & STORE & Q
```

Every arrow leaving `Core` crosses an interface the core declared
([ports and adapters](/principles/dependency-injection)). That is what makes the domain
testable without Postgres, Redis, or a network.

## The six queues at a glance

```mermaid
flowchart LR
    ING2["ingest"] -->|"new Job rows"| MATCH2["match"]
    ING2 --> ENR["enrich"]
    ING2 --> GHOST["ghost:score"]
    ING2 --> SAL["salary:infer"]
    MATCH2 -->|"user shortlists"| GEN2["generate"]
```

`ingest` is the fan-out point: persisting a new job triggers the per-job pipelines.
`generate` is the only queue driven by an explicit human action.

## Failure isolation

| Failure | Blast radius |
| --- | --- |
| One job source is down | that source's `ingest` task; `SourceRun.ok=false`, other sources unaffected |
| Cerebras key revoked | affected task fails terminally with the reason on its `ActivityRun`; Ollama tasks unaffected |
| Provider 429 | breaker holds that provider; queued tasks cancelled, retried by the operator later |
| Redis down | HTTP reads still serve from Postgres; enqueues fail |
| Postgres down | readiness fails (`/api/health/ready`); the process stays up |
| A worker goroutine wedges | the activity sweeper closes out its stale `ActivityRun` |

## Where to go next

- [Component map](/architecture/component-map) — every package and what it owns
- [Composition root](/architecture/composition-root) — how it is all wired
- [Request lifecycle](/architecture/request-lifecycle) — one request, end to end
- [Data flow](/architecture/data-flow) — one job, from board to kanban
