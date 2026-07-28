# Quickstart: validating 019-ai-job-throughput

**Feature**: 019-ai-job-throughput | Validates spec Success Criteria SC-001…SC-007.

## Prerequisites

- Stack up: `make up` (Postgres + Redis + API + dashboard).
- A default profile with a `rendercvConfig` (matching returns `ErrNoProfileConfig` otherwise).
- For hosted-path checks: `OLLAMA_KEY` set with `OLLAMA_URL=https://ollama.com`, or a
  `CEREBRAS_API_KEY` / `OPENROUTER_API_KEY`.
- For local-path checks: a local Ollama on `:11434` with the match model pulled.

Baselines for SC-001/SC-003 must be captured **before** implementation lands — see
"Capture the baseline" below. Record them in this feature directory as
`baseline.json`; the after-run comparison is meaningless without it.

## Capture the baseline (do this first, on the pre-change build)

1. Seed a fixed benchmark set of 50 jobs: `make seed` then note the job IDs, or reuse a
   real ingest run's output.
2. Enqueue matching for all 50 and time the drain end to end.
3. Record: total wall-clock, median per-job elapsed (`ActivityRun.elapsedMs`), and each
   job's `score` from `MatchResult`.

## Test suites

```bash
make test            # go test ./... + vitest
make test-integration  # Docker-backed Postgres/Redis paths
make test-lint       # required: this change spans apps/api, packages/shared, apps/dashboard
```

New coverage expected to pass:

- `internal/queue`: policy validation (concurrency ≥ 1, duration > 0,
  stale+sweep < 5m).
- `internal/llm`: `ProviderClass` resolution — local URL, loopback with key, cloud URL,
  Cerebras/OpenRouter with and without credential (falls back to Ollama ⇒ class follows the
  fallback).
- `internal/llm`: guard test asserting no provider client uses `retrieval.DefaultTransport`
  (FR-003).
- `internal/activity`: sweeper marks stale `running` → `interrupted`, leaves fresh rows and
  terminal rows untouched, and never re-opens a row that finished mid-sweep.
- worker middleware: deadline exceeded ⇒ run ends `timed_out` with elapsed recorded and no
  downstream enqueue (FR-008).
- `internal/matching`: profile-snapshot cache invalidates on profile update; embedding is
  skipped when the content hash is unchanged; concurrent match of the same job converges
  (FR-017).

## Scenario 1 — Hosted concurrency (SC-001, SC-002)

1. Set the `match` task to a hosted provider in dashboard Settings.
2. Enqueue the 50-job benchmark set.
3. While it drains: `curl localhost:3000/api/activity/queues | jq '.queues[] | select(.queue=="match")'`
   — expect `active: 3` and `concurrency: 3`.
4. Expect total wall-clock ≤ 40% of the recorded baseline and no increase in
   `failed`/`timed_out` counts (SC-006).

## Scenario 2 — Local concurrency unchanged (User Story 1, scenario 2)

1. Switch the `match` task back to a local Ollama model in Settings — no restart.
2. Enqueue 10 jobs; `active` for the `match` queue must never exceed
   `AI_CONCURRENCY_LOCAL` (default 1), and `providerClass` must read `local`.

## Scenario 3 — Deadline enforcement (SC-004, FR-008)

1. `AI_TASK_TIMEOUT_MATCH=10s` and restart the API.
2. Enqueue a match against a slow/blocked provider (point `OLLAMA_URL` at an unreachable
   host, or a stub that sleeps).
3. Within ~10s the run must read `timed_out`, `error` = `timed out after … (limit 10s)`,
   `finishedAt` set. No generation/salary task may be enqueued off that job.

## Scenario 4 — Crash recovery (SC-005, User Story 2)

1. Enqueue a batch; while several runs are `running`, `docker compose kill -s SIGKILL api`
   (SIGKILL, not `stop` — graceful shutdown is a different path).
2. Confirm in Postgres that rows are still `running` with a stale `heartbeatAt`.
3. `make up` again. Within 5 minutes **every** one of those rows must be terminal,
   `interrupted`, with `error` naming the heartbeat gap. Verify with:
   `select state, count(*) from "ActivityRun" where "createdAt" > now() - interval '1 hour' group by 1;`
4. Repeat with the API left running but a task wedged, to exercise the periodic sweep
   rather than the startup sweep.

## Scenario 5 — Existing ghost runs are cleaned (the reported symptom)

On the first deploy against the user's current database, rows stuck `running` for hours
have `heartbeatAt = null` and must be swept to `interrupted` on the startup sweep. Verify
the count before and after:

```sql
select count(*) from "ActivityRun" where state = 'running';
```

## Scenario 6 — Local match latency (SC-003)

1. Point the `match` task at the local model.
2. Re-run the 50-job benchmark set.
3. Median per-job elapsed must be ≤ 70% of baseline; every job's `score` must be within the
   agreed tolerance of its baseline score, with zero jobs crossing an AI-feature trigger
   threshold in either direction.
4. Second run over the same set must skip embedding recomputation (hash unchanged) —
   confirm via the absence of embed calls / a debug counter.

## Scenario 7 — Quota rejection under parallel load (FR-015)

Point at a provider whose quota is exhausted (or a stub returning `429` with
`Retry-After`). All three in-flight items must back off via the existing breaker, retry
per policy, and none may end `failed` on the first 429; the queue must not drain to
archived.

## Scenario 8 — UI states (SC-007)

Open the dashboard status page: `timed_out` and `interrupted` runs render distinctly from
`failed` and `cancelled`, each showing its reason, and both are retryable from the existing
retry control.
