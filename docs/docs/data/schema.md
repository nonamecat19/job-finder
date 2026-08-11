---
title: Schema
sidebar_position: 2
description: Every table, grouped by area, with ER diagrams and column detail.
---

# Schema

All identifiers except `host_retrieval_state` are quoted PascalCase/camelCase, inherited
from the initial Drizzle-generated migration (`00001_init.sql:1-5`).

## Jobs and sources

```mermaid
erDiagram
    JobSource ||--o{ SourceRun : "records runs"
    JobSource ||--o{ Subscription : "hosts"
    SavedSearch ||--o{ SourceRun : "triggers"
    Job ||--o| MatchResult : "scored by"
    Job ||--o| Application : "tracked as"
    Job ||--o{ GeneratedDocument : "produces"
    Job ||--o{ JobSignal : "analysed by"

    Job {
        uuid id PK
        text dedupeKey UK
        text sourceKey
        text externalId
        text title
        text company
        text location
        bool remote
        text salaryRaw
        text url
        text description
        jsonb raw
        timestamp postedAt
        timestamp ingestedAt
        vector embedding
        text status
    }
    JobSource {
        uuid id PK
        text key UK
        text kind
        bool enabled
        jsonb config
        bool healthy
    }
    SourceRun {
        uuid id PK
        uuid sourceId FK
        text searchId
        timestamp startedAt
        timestamp finishedAt
        bool ok
        int found
        int new
        text error
    }
    SavedSearch {
        uuid id PK
        text name
        jsonb query
        text cron
        bool enabled
        timestamp lastRunAt
    }
    Subscription {
        uuid id PK
        text sourceKey FK
        text name
        text url
        bool enabled
        timestamp lastRunAt
    }
```

| Column | Notes |
| --- | --- |
| `Job.dedupeKey` | `UNIQUE`; identity of a posting |
| `Job.raw` | `jsonb NOT NULL`, the untouched provider payload |
| `Job.embedding` | `vector(768)`, sized to `EMBED_DIMS` |
| `Job.status` | defaults to `'found'` |
| `JobSource.kind` | plain `text`, no CHECK; `api`, `scrape`, `sidecar` or `manual` in practice |
| `SavedSearch.cron` | defaults to `'0 */6 * * *'` |
| `Subscription.sourceKey` | FK to `JobSource.key`, cascade delete |

Indexes: `Job(ingestedAt)`, `Job(status)`, `MatchResult(score)`, `SourceRun(startedAt)`
(`00001_init.sql:108-111`).

## Matching, documents, profile

```mermaid
erDiagram
    Profile ||--o{ StarStory : "has"
    Profile ||--o{ FreshMatchNotification : "notified for"
    Job ||--o| MatchResult : "1:1"
    MatchResult ||--o{ FreshMatchNotification : "raises"
    Job ||--o{ GeneratedDocument : "versioned docs"

    Profile {
        uuid id PK
        text name
        jsonb document
        text extraNotes
        vector embedding
        text embedModel
        timestamp updatedAt
    }
    MatchResult {
        uuid id PK
        uuid jobId FK
        float similarity
        int score
        jsonb matchedSkills
        jsonb missingSkills
        text summary
        jsonb redFlags
        text model
    }
    GeneratedDocument {
        uuid id PK
        uuid jobId FK
        text type
        int version
        jsonb content
        text pdfPath
        text model
    }
    StarStory {
        uuid id PK
        uuid profileId
        text title
        text situation
        text task
        text action
        text result
        jsonb skills
        jsonb categories
    }
    FreshMatchNotification {
        uuid id PK
        uuid jobId FK
        uuid matchResultId FK
        uuid profileId FK
        bool fresh
        bool seen
    }
```

`GeneratedDocument` is unique on `(jobId, type, version)` — regeneration adds a version
rather than overwriting. `MatchResult` is unique on `jobId` — one current score per job.

## Application tracking

```mermaid
erDiagram
    Job ||--o| Application : "1:1"
    Application ||--o{ ApplicationOutcome : "outcomes"

    Application {
        uuid id PK
        uuid jobId FK
        text status
        text notes
        timestamp appliedAt
        jsonb events
        timestamp updatedAt
    }
```

`Application.events` is an append-only jsonb array defaulting to `'[]'` — the history of
status changes travels with the row rather than in a side table.

## AI and settings

```mermaid
erDiagram
    ResumeShapeSetting {
        text id PK
        int summaryLines
        bool skillsEnabled
        int skillsMaxGroups
        int experienceBulletsMin
        int experienceBulletsMax
        int targetPages
        bool projectsEnabled
        int projectsMin
        int projectsMax
        int projectBulletsMax
        bool certificationsEnabled
        int certificationsMin
        int certificationsMax
        timestamp updatedAt
    }
    AiFeatureSetting {
        text featureKey PK
        bool enabled
        int threshold
        timestamp updatedAt
    }
    AutoGenerateSetting {
        text id PK
        bool enabled
        int threshold
        timestamp updatedAt
    }
```

| Table | Constraint of note |
| --- | --- |
| `ResumeShapeSetting` | singleton: `CHECK (id = 'default')`. Every numeric column carries its own range `CHECK`, so an out-of-range shape cannot be stored even if application validation is bypassed. |
| `AiFeatureSetting` | keyed by feature, `threshold` defaults to 90 |
| `AutoGenerateSetting` | singleton: `CHECK (id = 'default')` |

`ResumeShapeSetting`'s defaults reproduce the shape the pipeline hardcoded before the
table existed (`00034_resume_shape_setting.sql`), so a fresh install and an upgraded
install generate identical documents. `0` means *unlimited* for the `max`/`min` columns.

**`LlmTaskSetting` no longer exists.** It held per-task `{provider, model}` until
`00033_drop_llm_task_setting.sql` dropped it; routing moved into `gateway/config.yaml`.

## Signals, analysis, salary

```mermaid
erDiagram
    Job ||--o{ JobSignal : "signals"
    Job ||--o{ KeywordDiff : "diffed"
    NormalizedTerm ||--o{ SynonymOverride : "aliases"

    JobSignal {
        uuid id PK
        uuid jobId FK
        text kind
        int score
        jsonb signals
        text model
    }
    SalaryCache {
        uuid id PK
        text bucket
        int salaryMin
        int salaryMax
        text currency
        text source
        int sampleSize
        timestamp updatedAt
    }
```

`JobSignal` is unique on `(jobId, kind)`, so the ghost detector's verdict for a job
replaces itself rather than accumulating. `SalaryCache` is unique on
`(bucket, currency, source)` with a `sampleSize` counter.

## Company and contacts

```mermaid
erDiagram
    Company ||--o{ CompanySignal : "signals"
    Contact ||--o{ ContactConnection : "from"
    Contact ||--o{ JobContact : "linked to jobs"
    Job ||--o{ JobContact : "has contacts"

    Company {
        uuid id PK
        text name
        text normalizedName UK
        text website
        timestamp firstSeenAt
        timestamp lastRefreshedAt
    }
    CompanySignal {
        uuid id PK
        uuid companyId FK
        text kind
        jsonb value
        text source
        jsonb raw
        timestamp fetchedAt
    }
    Contact {
        uuid id PK
        text name
        text email
        text company
        text role
        text linkedinUrl
        text githubUsername
        text source
    }
    ContactConnection {
        uuid id PK
        uuid fromContactId FK
        uuid toContactId FK
        text relationshipType
        real strength
    }
```

`ContactConnection` carries `CHECK (fromContactId <> toContactId)` — the graph cannot
contain a self-edge. `strength` is a `real` in `[0,1]`, defaulting to `0.5`, used to rank
referral paths.

## ATS board roster

```mermaid
erDiagram
    EmployerBoard {
        uuid id PK
        text vendor
        text employerIdentifier
        text displayName
        text addedVia
        bool enabled
        timestamp lastSuccessAt
        int lastPostingCount
        int consecutiveEmptyRuns
    }
    BoardCandidate {
        uuid id PK
        text vendor
        text employerIdentifier
        uuid inferredFromJobId
        text state
        timestamp decidedAt
    }
```

Both are unique on `(vendor, employerIdentifier)`. `BoardCandidate.state` starts at
`'proposed'` and is moved by `/api/roster/candidates/{id}/accept` or `/reject`;
`consecutiveEmptyRuns` on `EmployerBoard` is how a dead board is spotted.

## Operations

```mermaid
erDiagram
    ActivityRun {
        uuid id PK
        text op
        text state
        text label
        text step
        uuid jobId
        text sourceKey
        text queueTaskId
        text refId
        text error
        jsonb meta
        timestamp createdAt
        timestamp startedAt
        timestamp finishedAt
    }
    host_retrieval_state {
        uuid id PK
        text host UK
        text identity_version
        text current_rung
        timestamptz rung_last_verified_at
        jsonb cookies
        int consecutive_blocks
        timestamptz cooling_off_until
        timestamptz last_block_at
        text last_block_reason
        int crawl_delay_seconds
    }
```

| Column | Meaning |
| --- | --- |
| `ActivityRun.op` | operation name, e.g. the task type |
| `ActivityRun.state` | `queued` by default, then `running`, `succeeded`, `failed`, `cancelled` |
| `ActivityRun.queueTaskId` | asynq task id, used by cancel/retry and the sweeper |
| `ActivityRun.meta` | free-form jsonb, defaults to `'{}'` |
| `host_retrieval_state.cookies` | encrypted with `CONFIG_ENCRYPTION_KEY` |
| `host_retrieval_state.current_rung` | `direct`, `browser`, or `flaresolverr` |
| `host_retrieval_state.crawl_delay_seconds` | learned from the host's `robots.txt` |

:::note Vestigial budget columns
`budget_period_start`, `budget_used` and `budget_limit` remain on
`host_retrieval_state` from the original design. Migration `00029_drop_host_budget.sql`
removed the per-host daily budget behaviour in favour of crawl-delay-aware pacing.
:::
