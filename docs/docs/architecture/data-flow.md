---
title: Data flow
sidebar_position: 5
description: One job, followed from a board listing to a tracked application.
---

# Data flow

## The whole pipeline

```mermaid
flowchart TD
    CRON["Scheduler tick or manual run"] --> ENQ["enqueue ingest with sourceKey and optional searchId"]
    ENQ --> AD["Adapter fetches via retrieval ladder"]
    AD --> NORM["Normalize to NormalizedJob"]
    NORM --> DEDUP{"dedupeKey seen?"}
    DEDUP -->|yes| BUMP["update seen count, skip"]
    DEDUP -->|no| INS["INSERT Job with status=found and raw payload kept"]
    INS --> RUN["SourceRun: found and new counters"]
    INS --> FAN["fan-out"]
    FAN --> M["match"]
    FAN --> E["enrich"]
    FAN --> G["ghost:score"]
    FAN --> S["salary:infer"]
    M --> MR["MatchResult: similarity, score, skills, red flags"]
    E --> DETAIL["full description backfilled onto Job"]
    G --> SIG["JobSignal rows"]
    S --> SC["SalaryCache entry"]
    MR --> NOTIF{"score above MATCH_NOTIFY_SCORE_THRESHOLD?"}
    NOTIF -->|yes| FMN["FreshMatchNotification"]
    MR --> FEED["Feed ranking"]
```

## Stage by stage

| Stage | Package | Writes |
| --- | --- | --- |
| Schedule | `ingestion` (`scheduler.go`) | enqueues `ingest` |
| Fetch | `jobsources/adapters` + `retrieval` | `host_retrieval_state` |
| Normalize + dedupe | `ingestion` (`dedupe.go`) | `Job`, `SourceRun` |
| Score | `matching` | `MatchResult` |
| Detail backfill | `enrichment` | `Job.description`, `Job.raw` |
| Ghost detection | `ghostjob` | `JobSignal` |
| Salary inference | `salary` | `SalaryCache` |
| Notify | `notifier` | `FreshMatchNotification` |
| Generate | `generation` | `GeneratedDocument` + PDF in storage |
| Track | `applications` | `Application` (status, events) |

## Embeddings and similarity

```mermaid
sequenceDiagram
    participant P as Profile
    participant O as Ollama EMBED_MODEL
    participant J as Job
    participant DB as Postgres and pgvector
    participant M as matching
    P->>O: embed profile document
    O-->>DB: Profile.embedding vector 768
    J->>O: embed job description
    O-->>DB: Job.embedding vector 768
    M->>DB: cosine similarity vs MATCH_SIMILARITY_THRESHOLD
    DB-->>M: candidate jobs
    M->>M: LLM fit scoring on candidates
    M->>DB: MatchResult
```

:::note Embeddings stay on Ollama
Chat tasks can be routed to Cerebras; embeddings cannot — neither remote provider exposes
an embeddings API. `EMBED_URL` therefore always points at an Ollama instance
(`internal/llm/factory.go:24-30`).
:::

The two-phase design — cheap vector recall, then expensive LLM scoring on the survivors —
is what keeps a local model viable for a feed of thousands of jobs.

## Document generation

```mermaid
sequenceDiagram
    participant U as User
    participant API as POST /api/jobs/id/generate
    participant Q as generate queue
    participant G as generation.Service
    participant L as llm.Router generation
    participant RC as RenderCV
    participant ST as storage
    participant DB as GeneratedDocument
    U->>API: request resume or cover letter
    API->>Q: enqueue GeneratePayload
    Q->>G: ProcessTask
    G->>DB: read Profile, Job, MatchResult
    G->>L: grounded generation per RESUME_GROUNDING_LEVEL
    L-->>G: structured document JSON
    G->>RC: render PDF via RENDERCV_BIN
    RC-->>ST: pdfPath
    G->>DB: insert GeneratedDocument row
```

`GeneratedDocument` is versioned by `UNIQUE(jobId, type, version)`
(`00001_init.sql:19-28`) — regenerating never overwrites the copy you already sent.

## Activity as the cross-cutting spine

Every async operation writes an `ActivityRun`, which is what the Status page reads.

```mermaid
stateDiagram-v2
    [*] --> queued
    queued --> running
    running --> running: heartbeat
    running --> succeeded
    running --> failed
    running --> cancelled
    queued --> failed: queued grace exceeded
    succeeded --> [*]
    failed --> [*]
    cancelled --> [*]
```

## Read path for the feed

```mermaid
flowchart LR
    UI["FeedPage"] --> API2["GET /api/jobs"]
    API2 --> J[("Job")]
    API2 --> MR2[("MatchResult")]
    API2 --> A2[("Application")]
    J --> DTO["Job list DTO"]
    MR2 --> DTO
    A2 --> DTO
    DTO --> UI
```

The join happens in SQL (`internal/db/queries/joblist.sql`) so the dashboard renders one
paginated payload rather than issuing per-job follow-ups.
