# Phase 1 Data Model: AI Job Throughput & Stuck-Run Recovery

**Feature**: 019-ai-job-throughput | **Date**: 2026-07-28

---

## 1. ActivityRun (existing table, extended)

Source: `apps/api/internal/db/migrations/00004_activity_run.sql`.

### New columns (migration `00030_activity_run_liveness.sql`)

| Column | Type | Null | Default | Purpose |
|---|---|---|---|---|
| `heartbeatAt` | `timestamp (3)` | yes | `null` | Last proof of life from the owning worker. Written on start, on every step, and by the worker heartbeat ticker. `null` on a `queued` row. |
| `timeoutMs` | `integer` | yes | `null` | The deadline the run was admitted under, copied from the task-type policy at start. Lets the sweeper and the UI explain *what* limit was exceeded. |

Index: `CREATE INDEX "ActivityRun_heartbeat_idx" ON "ActivityRun" ("heartbeatAt") WHERE "state" = 'running';`
— the sweeper's only query pattern.

No backfill: existing `running` rows have `heartbeatAt = null`, which the sweeper treats as
stale (see §1.2), so the user's current 10-hour ghosts are closed out on the first sweep
after deploy.

### 1.1 State set (extended)

`state` is plain `text` with no CHECK constraint, so the values below are additive.

| State | Terminal | Written by | Meaning |
|---|---|---|---|
| `queued` | no | `InsertActivityRun` | enqueued, not yet picked up |
| `running` | no | `StartActivityRun` | a worker owns it and is heartbeating |
| `succeeded` | yes | `FinishActivityRunOk` | completed |
| `failed` | yes | `FinishActivityRunError` | handler returned an error |
| `cancelled` | yes | `FinishActivityRunCancelled` | user cancelled, or skipped on an upstream rate limit |
| **`timed_out`** | yes | `FinishActivityRunTimedOut` (new) | exceeded its task-type deadline; detected in-process |
| **`interrupted`** | yes | `SweepStaleActivityRuns` (new) | worker vanished (crash/power loss) or queued task no longer exists |

### 1.2 Transitions

```
queued ──start──> running ──ok──────────> succeeded
   │                 │ ├──error────────> failed
   │                 │ ├──cancel───────> cancelled
   │                 │ ├──deadline─────> timed_out      (in-process, ctx.DeadlineExceeded)
   │                 │ └──stale heartbeat──> interrupted (sweeper)
   └──no live asynq task + past grace──────> interrupted (sweeper)
```

Rules:

- Terminal → terminal transitions are not performed by the sweeper: `SweepStaleActivityRuns`
  filters `state = 'running'` (or `'queued'`), so a run that finished between sweep and
  write is never re-opened.
- A late in-process finish that lands *after* the sweeper marked a run `interrupted`
  overwrites it with the real outcome. Last writer wins, deliberately: a run that actually
  succeeded should read `succeeded` (covers the laptop-suspend edge case).
- `finishedAt` is set on every terminal transition; `error` carries a human-readable reason
  for `timed_out` / `interrupted`.

### 1.3 Reason strings

| State | `error` format | Example |
|---|---|---|
| `timed_out` | `timed out after {elapsed} (limit {limit})` | `timed out after 5m0s (limit 5m0s)` |
| `interrupted` | `interrupted: no worker heartbeat for {stale}` | `interrupted: no worker heartbeat for 14h2m` |
| `interrupted` (queued) | `interrupted: queued task no longer exists` | — |

---

## 2. TaskPolicy (new, in-process config value)

Not persisted. Built once at startup from `config.Config`, one per AI task type. Owned by
`apps/api/internal/queue` so both `cmd/server` and handlers can read it.

| Field | Type | Source | Default |
|---|---|---|---|
| `TaskType` | string | constant | — |
| `Queue` | string | constant | — |
| `LocalConcurrency` | int | `AI_CONCURRENCY_LOCAL` | 1 |
| `HostedConcurrency` | int | `AI_CONCURRENCY_CLOUD` | 3 |
| `MaxDuration` | time.Duration | `AI_TASK_TIMEOUT_<TYPE>` | see research R9 |
| `LLMTaskKey` | string | existing llmsettings task key; empty for non-LLM tasks | — |

Validation: concurrencies ≥ 1; `MaxDuration` > 0; pool size = `max(Local, Hosted)`.
Non-LLM tasks (`ingest`, `enrich`) have an empty `LLMTaskKey` and use a single fixed
concurrency (their current value) — only the deadline applies to them.

---

## 3. ProviderClass (new, derived at call time)

`llm.ProviderClass` ∈ {`local`, `hosted`}, resolved from the live `RouterSnapshot`:

```
resolve(taskKey):
  setting := snapshot.Tasks[taskKey]
  cerebras (with credential)   -> hosted
  openrouter (with credential) -> hosted
  otherwise (ollama, or a remote provider that fell back) ->
      hosted if OllamaProvider.apiKey != "" or baseURL host is not loopback/private
      local  otherwise
```

Exposed as `(*llm.Router).ProviderClass() ProviderClass`, mirroring the existing
`resolve()` so the fallback-to-Ollama path (`router.go:88-107`) yields the *effective*
class, not the requested one.

---

## 4. Admission gate (new, in-process)

One `gate` per AI task type, holding two counting semaphores:

| Semaphore | Capacity | Acquired when |
|---|---|---|
| `hosted` | `TaskPolicy.HostedConcurrency` | resolved class is `hosted` |
| `local` | `TaskPolicy.LocalConcurrency` | resolved class is `local` |

Acquired inside the worker middleware, before the handler runs; released on return. Class
is resolved once per task and does not change mid-flight. Acquisition is ctx-aware, so a
task waiting for a slot still honours its deadline and shutdown.

---

## 5. Profile snapshot cache (new, in-process)

Removes the per-job repeat work identified in research R5.

| Field | Type | Notes |
|---|---|---|
| `ProfileID` | string | default profile |
| `ProfileText` | string | `RendercvToText(master) + extraNotes`, pre-truncated to 6000 |
| `HasEmbedding` | bool | folded in, so matching stops re-checking per job |
| `Version` | timestamp | profile `updatedAt`; a newer value invalidates the entry |

Read path: `matching.Service` asks the cache; on miss or version change it rebuilds from
`profiles.GetDefault`. Invalidated on any profile write and on `RefreshEmbedding`.

---

## 6. Job embedding skip (existing columns, new guard)

`Job.embedding` already exists. Add `Job.embeddingHash` (`text`, nullable) in the same
migration: the hash of the exact text passed to `Embed` (`title|company|description`
truncated to 8000, per `scoring.go:26`). Matching recomputes the embedding only when the
stored hash differs or is null. `null` = always recompute, so existing rows behave exactly
as today until their first re-match.

---

## 7. Queue backlog view (new, read-only projection)

Not stored. Assembled per request from `asynq.Inspector.GetQueueInfo` per queue plus the
DB count of active runs:

| Field | Type | Source |
|---|---|---|
| `queue` | string | queue name |
| `pending` / `active` / `scheduled` / `retry` / `archived` | int | `asynq.QueueInfo` |
| `concurrency` | int | effective admission capacity for the current provider class |
| `providerClass` | `"local"`\|`"hosted"`\|`null` | resolved class; null for non-LLM queues |
| `processedPerMinute` | float | `asynq.QueueInfo` processed counters |
| `etaSeconds` | int \| null | `pending / processedPerMinute`; null when throughput is 0 |
