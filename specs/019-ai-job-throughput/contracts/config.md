# Contract: Configuration surface

**Feature**: 019-ai-job-throughput

New env keys, registered in `apps/api/internal/config/defaults.go` (`defaults` map) and
read through `config.Config` struct tags. All have defaults; none are required (FR-005).
Documented in `.env.example` alongside the existing LLM block.

## Concurrency

| Key | Type | Default | Effect |
|---|---|---|---|
| `AI_CONCURRENCY_CLOUD` | int | `3` | Simultaneous AI items per task type when that task resolves to a hosted provider (Cerebras, OpenRouter, or Ollama Cloud). |
| `AI_CONCURRENCY_LOCAL` | int | `1` | Simultaneous AI items per task type when the task resolves to a local Ollama. Preserves today's behaviour. |
| `INGEST_CONCURRENCY` | int | `2` | Unchanged behaviour, promoted from a hardcoded literal. |
| `ENRICH_CONCURRENCY` | int | `1` | Unchanged behaviour, promoted from a hardcoded literal. Deliberately low: authenticated per-job page fetches. |

Validation: values `< 1` are rejected at startup with a config error naming the key.
The asynq pool for an LLM task type is sized `max(AI_CONCURRENCY_CLOUD, AI_CONCURRENCY_LOCAL)`;
the admission gate enforces the applicable one at run time.

## Deadlines

| Key | Type | Default | Applies to |
|---|---|---|---|
| `AI_TASK_TIMEOUT_MATCH` | duration | `5m` | `match` |
| `AI_TASK_TIMEOUT_GENERATE` | duration | `15m` | `generate` |
| `AI_TASK_TIMEOUT_SALARY` | duration | `5m` | `salary:infer` |
| `AI_TASK_TIMEOUT_GHOST` | duration | `5m` | `ghost:score` |
| `AI_TASK_TIMEOUT_ENRICH` | duration | `10m` | `enrich` |
| `AI_TASK_TIMEOUT_INGEST` | duration | `30m` | `ingest` |

Parsed with `time.ParseDuration`; `<= 0` rejected at startup.

## Liveness / sweeper

| Key | Type | Default | Effect |
|---|---|---|---|
| `ACTIVITY_HEARTBEAT_INTERVAL` | duration | `30s` | How often a running worker refreshes `ActivityRun.heartbeatAt`. |
| `ACTIVITY_STALE_AFTER` | duration | `2m` | A `running` row with no heartbeat for this long is `interrupted`. Must be ≥ 2× heartbeat interval; enforced at startup. |
| `ACTIVITY_SWEEP_INTERVAL` | duration | `1m` | Sweeper period. A sweep also runs once at startup. |
| `ACTIVITY_QUEUED_GRACE` | duration | `30m` | A `queued` row older than this with no live asynq task is `interrupted`. |

`ACTIVITY_STALE_AFTER + ACTIVITY_SWEEP_INTERVAL` must stay under 5 minutes to satisfy
FR-009/SC-005; enforced by a startup validation and a unit test.

## Local model performance

| Key | Type | Default | Effect |
|---|---|---|---|
| `OLLAMA_KEEP_ALIVE` | string | `30m` | Sent as `keep_alive` on Ollama chat and embed requests so a local model stays resident across a queue drain. Ignored by Ollama Cloud. Empty string omits the field entirely. |
| `LLM_MAX_IDLE_CONNS_PER_HOST` | int | `4` | Idle-connection pool for LLM provider clients. Go's default of 2 forces a fresh TLS handshake on the third concurrent hosted request. |

## Non-goals

- No change to `DJINNI_RATE_OVERRIDE_RPS` or any `ratelimit`/`retrieval` pacing key. AI
  provider traffic does not pass through the paced transport and must not start doing so
  (FR-003, guarded by test).
