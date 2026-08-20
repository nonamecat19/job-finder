---
title: Configuration reference
sidebar_position: 5
description: Every environment variable — purpose, default, whether it is required, and which component reads it.
---

# Configuration reference

Source of truth: `apps/api/internal/config/config.go` (field tags) and `.env.example`
(defaults and comments).

```mermaid
flowchart LR
    F[".env"] --> MK["Justfile includes and exports it"]
    F --> DC["docker compose substitution"]
    MK --> GO["config.Load()"]
    DC --> SVC["container environment"]
    GO --> VAL["queue.PoliciesFromConfig — validates"]
    VAL --> RUN["running process"]
```

## Core

| Variable | Purpose | Default | Required |
| --- | --- | --- | --- |
| `PORT` | API listen port | `3000` | no |
| `DATABASE_URL` | Postgres DSN | — | **yes** — `main.run` errors without it |
| `REDIS_URL` | asynq Redis | `redis://localhost:6379` | no |
| `DB_PASSWORD` | used by compose and the DSN | — | **yes** for compose |
| `TEST_DATABASE_URL` | test database | see `.env.example` | for tests |
| `TEST_REDIS_URL` | test Redis (DB index 1) | `redis://localhost:6379/1` | for tests |
| `CONFIG_ENCRYPTION_KEY` | AES-256-GCM key, 32 bytes hex | empty (dev fallback: plaintext) | production |

Generate the key with `openssl rand -hex 32`. See
[configuration](/backend/configuration) for rotation caveats.

## Inference

| Variable | Purpose | Default |
| --- | --- | --- |
| `OLLAMA_URL` | Ollama endpoint | `https://ollama.com` in `.env.example` |
| `OLLAMA_KEY` | Bearer token for Ollama Cloud | empty (local needs none) |
| `EMBED_URL` | embeddings endpoint | blank = same as `OLLAMA_URL` |
| `EMBED_MODEL` | embedding model | `nomic-embed-text` |
| `EMBED_DIMS` | vector dimensions | `768` — must match the `vector(768)` columns |
| `OLLAMA_KEEP_ALIVE` | keep the model resident | `30m` |
| `LLM_MAX_IDLE_CONNS_PER_HOST` | LLM transport tuning | `4` |

:::warning `EMBED_URL` with Ollama Cloud
Ollama Cloud serves no embedding models. If `OLLAMA_URL` is the cloud, point `EMBED_URL` at
a local Ollama, e.g. `http://localhost:11434`.
:::

## Models per task

| Variable | Task key | Default in `.env.example` |
| --- | --- | --- |
| `LLM_MODEL` | fallback for all tasks | `gpt-oss:120b-cloud` |
| `LLM_MODEL_MATCH` | `match` | `gpt-oss:20b-cloud` |
| `LLM_MODEL_GENERATION` | `generation` | `gpt-oss:120b-cloud` |
| `LLM_MODEL_REPHRASE` | `rephrase` | `gpt-oss:120b-cloud` |
| `LLM_MODEL_GHOST` | `ghost` | `gpt-oss:120b-cloud` |

Empty falls back to `LLM_MODEL`. These apply on the **direct-Ollama path only** — when
`GATEWAY_URL` is set, the Router sends the task key as the model and the gateway resolves
it from `gateway/config.yaml`.

## LiteLLM gateway

| Variable | Purpose | Default |
| --- | --- | --- |
| `GATEWAY_URL` | proxy endpoint, e.g. `http://litellm:4000` | empty = no gateway; every task talks to Ollama directly |
| `LITELLM_MASTER_KEY` | authenticates the app to the proxy | required whenever `GATEWAY_URL` is set |

### Provider keys

These are consumed **inside the `litellm` container only**. The Go backend never reads
them, and none of them may be set in the api service's environment.

| Variable | Provider |
| --- | --- |
| `GROQ_API_KEY` | Groq |
| `COHERE_API_KEY` | Cohere |
| `OPENROUTER_API_KEY` | OpenRouter |

An absent key does not prevent startup and does not cause a request-time error — that
entry is skipped and the chain advances. Every chain terminates at Ollama, so AI work
completes even with none of these set.

Changing which model serves a task is an edit to `gateway/config.yaml` followed by
`docker compose restart litellm` — no application restart. Adding or changing a key is
also a `litellm` restart.

## Matching and notifications

| Variable | Purpose | Default |
| --- | --- | --- |
| `MATCH_SIMILARITY_THRESHOLD` | prefilter cutoff | `0.35` |
| `MATCH_NOTIFY_SCORE_THRESHOLD` | minimum score to notify | service default |
| `MATCH_NOTIFY_RATE_LIMIT` | notification cap per window | service default |
| `KEYWORD_REPHRASE_CACHE_TTL_SEC` | rephrase cache lifetime | `900` |

## Throughput and deadlines

| Variable | Purpose | Default |
| --- | --- | --- |
| `AI_CONCURRENCY_CLOUD` | concurrent AI tasks on a hosted provider | `3` |
| `AI_CONCURRENCY_LOCAL` | concurrent AI tasks on local Ollama | `1` |
| `INGEST_CONCURRENCY` | ingest workers | `2` |
| `ENRICH_CONCURRENCY` | enrich workers | `1` |
| `AI_TASK_TIMEOUT_MATCH` | match deadline | `5m` |
| `AI_TASK_TIMEOUT_GENERATE` | generate deadline | `15m` |
| `AI_TASK_TIMEOUT_SALARY` | salary deadline | `5m` |
| `AI_TASK_TIMEOUT_GHOST` | ghost deadline | `5m` |
| `AI_TASK_TIMEOUT_ENRICH` | enrich deadline | `10m` |
| `AI_TASK_TIMEOUT_INGEST` | ingest deadline | `30m` |

All are validated at startup: concurrency `>= 1`, durations `> 0`.

## Activity liveness

| Variable | Purpose | Default |
| --- | --- | --- |
| `ACTIVITY_HEARTBEAT_INTERVAL` | worker heartbeat cadence | `30s` |
| `ACTIVITY_STALE_AFTER` | no heartbeat → interrupted | `2m` |
| `ACTIVITY_SWEEP_INTERVAL` | sweeper cadence | `1m` |
| `ACTIVITY_QUEUED_GRACE` | queued with no task → interrupted | `30m` |

Constraints enforced at boot: `ACTIVITY_STALE_AFTER >= 2 × ACTIVITY_HEARTBEAT_INTERVAL`,
and `ACTIVITY_STALE_AFTER + ACTIVITY_SWEEP_INTERVAL < 5m`.

## Retrieval

| Variable | Purpose | Default |
| --- | --- | --- |
| `FLARESOLVERR_URL` | enables the top ladder rung | empty = rung absent |
| `BROWSER_IDENTITY_VERSION` | identity profile | `chrome126` |
| `COOLING_OFF_THRESHOLD` | consecutive blocks before cooling off | config default |
| `COOLING_OFF_BASE_DURATION` | cooling-off length | config default |
| `CHEAP_RUNG_RETEST_INTERVAL` | when to retry a cheaper rung | config default |
| `DJINNI_RATE_OVERRIDE_RPS` | per-host rate override for `djinni.co` | unset = `DefaultRPS` (0.7) |
| `DJINNI_DETAIL_DELAY_MS` | delay between Djinni detail fetches | `1500` |
| `WORKUA_DETAIL_DELAY_MS` | same for work.ua | config default |

## Source credentials

| Variable | Source | Notes |
| --- | --- | --- |
| `ADZUNA_APP_ID`, `ADZUNA_APP_KEY` | `adzuna` | API credentials |
| `ADZUNA_COUNTRY` | `adzuna` | market, default `gb` |
| `JOOBLE_API_KEY` | `jooble` | |
| `JOBLEADS_EMAIL`, `JOBLEADS_PASSWORD` | `jobleads` | login-gated; never escalates past the direct rung |
| `LINKEDIN_SCRAPE_ENABLED` | recruiter resolution | `false` — ToS gray area; other contact sources always run |

## Documents and storage

| Variable | Purpose | Default |
| --- | --- | --- |
| `DOCUMENTS_DIR` | local document path | `./data/documents` |
| `RESUME_GROUNDING_LEVEL` | how far tailoring may depart from the master | `moderate` |
| `RENDERCV_BIN` | **Obsolete** — loaded but not read; rendering is in-process via `rendercv-go` | `rendercv` |
| `MINIO_ENDPOINT` | `host:port`, no scheme | empty = MinIO disabled |
| `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | credentials | `minioadmin` |
| `MINIO_BUCKET` | bucket | `documents` |
| `MINIO_USE_SSL` | TLS | `false` |
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | compose-only, for the MinIO container | `minioadmin` |

## Salary

| Variable | Purpose | Default |
| --- | --- | --- |
| `LEVELS_FYI_CSV` | levels.fyi dataset path | empty = source disabled with a warning |
| `SALARY_FLOOR_USD` | minimum annual USD filter | `0` = disabled |

## Compose-only

| Variable | Consumed by |
| --- | --- |
| `COMPOSE_PROJECT_NAME` | derived per worktree by the Justfile |
| `POSTGRES_HOST_PORT` | derived per worktree; `5432 + (hash % 100)` |
| `DB_PASSWORD` | `${DB_PASSWORD:?set DB_PASSWORD in .env}` — compose fails without it |

## Minimum viable `.env`

```bash
DB_PASSWORD=change-me
DATABASE_URL=postgresql://jobfinder:change-me@localhost:5432/jobfinder
REDIS_URL=redis://localhost:6379
CONFIG_ENCRYPTION_KEY=<openssl rand -hex 32>
OLLAMA_URL=http://localhost:11434
EMBED_URL=http://localhost:11434
EMBED_MODEL=nomic-embed-text
LLM_MODEL=<a model you have pulled>
```

Everything else has a working default or disables a feature cleanly.
