---
title: Runbooks
sidebar_position: 6
description: Symptom → checks → fix, for the failures this system actually produces.
---

# Runbooks

## Triage entry point

```mermaid
flowchart TD
    S["Something is wrong"] --> H{"GET /api/health responds?"}
    H -->|no| R1["Process down — see 'API will not start'"]
    H -->|yes| RD{"GET /api/health/ready ok?"}
    RD -->|no| R2["Read the checks object — dependency down"]
    RD -->|yes| W{"What is missing?"}
    W -->|"no new jobs"| R3["Ingestion stalled"]
    W -->|"no scores"| R4["AI pipeline stalled"]
    W -->|"no documents"| R5["Generation failing"]
    W -->|"UI shows undefined"| R6["Contract drift"]
```

## API will not start

**Symptoms.** Process exits immediately.

**Checks.**

```bash
just run-backend 2>&1 | head -20
```

| Message | Cause | Fix |
| --- | --- | --- |
| `DATABASE_URL is required` | missing env | set it in `.env` |
| `db: connect:` / `db: ping:` | Postgres unreachable | `just up`, check `POSTGRES_HOST_PORT` |
| `db: migrate up:` | migration failed | see the migration runbook below |
| `queue: <task>: local concurrency must be >= 1` | bad config | fix the concurrency variable |
| `queue: ACTIVITY_STALE_AFTER ...` | liveness bounds violated | stale ≥ 2 × heartbeat; stale + sweep < 5m |
| `queue: invalid REDIS_URL` | malformed URL | `redis://host:port/db` |

All of these are startup validation working as designed — the process refuses to run
misconfigured rather than misbehaving under load.

## Ingestion stalled — no new jobs

```mermaid
flowchart TD
    A["No new jobs"] --> B["GET /api/searches/runs/recent"]
    B --> C{"recent SourceRun rows?"}
    C -->|no| D["scheduler not firing"]
    C -->|yes| E{"ok = false?"}
    E -->|yes| F["read SourceRun.error"]
    E -->|no| G{"found > 0 but new = 0?"}
    G -->|yes| H["everything is a duplicate — expected"]
    D --> D1["check SavedSearch.enabled and cron"]
    D --> D2["logs: 'scheduler: bad cron expression'"]
    F --> F1["blocked → GET /api/hosts/{host}/retrieval-status"]
    F --> F2["auth → source credentials"]
```

**Fixes.**

| Cause | Action |
| --- | --- |
| Search disabled or bad cron | fix it on the Sources page; `cron.ParseStandard` syntax |
| Host cooling off | `POST /api/hosts/{host}/override-cooling-off`, or wait |
| Host pinned to a broken rung | `POST /api/hosts/{host}/clear-rung-preference` |
| Stale session cookies | `POST /api/hosts/{host}/clear-cookies` |
| Credentialed source failing login | check `JOBLEADS_EMAIL` / `JOBLEADS_PASSWORD`; it can only use the direct rung |
| Adapter broken by a site change | run the source's live smoke test |
| Just slow | pacing is 0.7 rps per host by design |

## AI pipeline stalled — no scores

```mermaid
flowchart TD
    A["Jobs arrive, no MatchResult"] --> B["GET /api/activity"]
    B --> C{"match runs present?"}
    C -->|no| D["not enqueued — check the ingest handler's fan-out"]
    C -->|"queued, never running"| E["GET /api/activity/queues — concurrency saturated"]
    C -->|failed| F["read the error"]
    C -->|cancelled| G["provider rate limited — breaker held"]
    C -->|"succeeded, score 0"| H["'no profile config' → configure a profile"]
```

| Error | Meaning | Fix |
| --- | --- | --- |
| `llm: provider credential rejected` | bad key | fix the key, **restart** (credentials are env-only) |
| `llm: provider account out of credits` | quota | top up, or switch the task to Ollama |
| `llm: model unavailable` | unknown model id | fix the entry in `gateway/config.yaml`, then `docker compose restart litellm` |
| `llm: provider unavailable` | 5xx / transport | transient; check the provider |
| `llm: rate limited` | 429 | breaker holds up to 15 minutes; retry from the Status page |
| `queue: task exceeded its deadline` | slow model | raise `AI_TASK_TIMEOUT_MATCH` or route the task to a faster model |

:::note Work is running on Ollama when you expected a hosted provider
Every task chain terminates at Ollama by design, and entries whose credential is absent
are skipped silently. Check `docker compose logs litellm` for which upstream answered, and
confirm the provider key is present in the **`litellm` service's** environment — the Go
backend never reads provider keys. With `GATEWAY_URL` unset, everything runs on Ollama.
:::

## Queue backlog

```mermaid
sequenceDiagram
    participant You
    participant Q as /api/activity/queues
    participant AM as asynqmon :8090
    You->>Q: backlog per queue + provider class
    You->>AM: active vs pending vs retry
    alt active = 0, pending > 0
        Note over You: workers not consuming — is the process up?
    else active = concurrency limit
        Note over You: saturated — raise AI_CONCURRENCY_CLOUD or switch provider
    else retry count climbing
        Note over You: read the task error in asynqmon
    end
```

Recovery actions: `POST /api/activity/retry`, `POST /api/activity/cancel-all`, or archive
in asynqmon.

## Runs stuck in `running`

**Cause.** The worker died without finalising its run.

**What happens automatically.** The sweeper marks it `interrupted` within
`ACTIVITY_STALE_AFTER + ACTIVITY_SWEEP_INTERVAL` — three minutes on defaults. It also
sweeps once at startup, so a restart cleans up the previous process's orphans.

**If it does not clear:** check the process is actually running, and that the sweeper
goroutine started (`runServers` launches it alongside the scheduler).

## Migration failure

```mermaid
flowchart TD
    A["db: migrate up: ..."] --> B["read the SQL error"]
    B --> C{"constraint violation?"}
    C -->|yes| D["existing data violates a new constraint"]
    C -->|no| E{"already exists?"}
    E -->|yes| F["partially applied — check goose_db_version"]
    D --> D1["write a backfill migration first, then the constraint"]
    F --> F1["dev: just clean and start over; prod: fix forward"]
```

Never edit an applied migration. Add a new one.

## Contract drift — UI shows `undefined`

```mermaid
flowchart TD
    A["Field is undefined in the dashboard"] --> B{"present in the API response?"}
    B -->|no| C["handler does not map it, or the DTO lacks a JSON tag"]
    B -->|yes| D{"in packages/shared/src/index.ts?"}
    D -->|no| E["mirror the field by hand"]
    D -->|yes| F["rebuild shared: pnpm --filter @job-finder/shared build"]
```

Verify the wire directly:

```bash
curl -s localhost:3000/api/jobs | head -c 500
```

Remember: `tygo-check` guards `generated.ts`; **nothing** guards `index.ts`.

## Documents not generating

| Check | Action |
| --- | --- |
| `/api/activity` for `generate` runs | read the error |
| `rendercv` on PATH | `RENDERCV_BIN`; try it manually |
| `DOCUMENTS_DIR` writable | check permissions |
| MinIO configured but down | readiness shows it; either fix it or unset `MINIO_ENDPOINT` |
| Deadline exceeded | raise `AI_TASK_TIMEOUT_GENERATE` (default 15m) |

## Everything is slow

| Suspect | Check | Lever |
| --- | --- | --- |
| Local model | Ollama's own load | smaller model, or move the task to Cerebras |
| Hosted concurrency | `/api/activity/queues` | `AI_CONCURRENCY_CLOUD`, `LLM_MAX_IDLE_CONNS_PER_HOST` |
| Scraping pacing | by design at 0.7 rps/host | do not raise it for third-party hosts |
| Postgres | readiness latency figures | indexes exist on `Job(ingestedAt)`, `Job(status)`, `MatchResult(score)` |
| Feed rendering | thousands of rows | already virtualised |

## Redis flushed

Queued `ActivityRun` rows have no task behind them. The sweeper's queued pass confirms
absence via the asynq inspector and marks them `interrupted` after
`ACTIVITY_QUEUED_GRACE` (30 minutes). Re-run the affected searches; nothing in Postgres is
lost — Redis holds only in-flight work.

## Full reset (development)

```bash
just clean            # down -v, remove node_modules and dist
just up
pnpm install
pnpm --filter @job-finder/shared build
just run-backend      # migrates a fresh database
just seed             # optional sample data
```
