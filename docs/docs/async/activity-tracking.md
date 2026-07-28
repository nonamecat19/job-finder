---
title: Activity tracking
sidebar_position: 4
description: ActivityRun, the recorder API, heartbeats, the sweeper, and the activity HTTP surface.
---

# Activity tracking

`ActivityRun` is how every async operation becomes visible. One row per task attempt, from
enqueue to terminal state.

## The row

```sql
CREATE TABLE "ActivityRun" (
  "id"          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "op"          text NOT NULL,
  "state"       text NOT NULL DEFAULT 'queued',
  "label"       text NOT NULL,
  "step"        text,
  "jobId"       uuid,
  "sourceKey"   text,
  "queueTaskId" text,
  "refId"       text,
  "error"       text,
  "meta"        jsonb NOT NULL DEFAULT '{}'::jsonb,
  "createdAt"   timestamp (3) DEFAULT now() NOT NULL,
  "startedAt"   timestamp (3),
  "finishedAt"  timestamp (3)
);
```

| Column | Role |
| --- | --- |
| `op` | which operation — used to find the asynq queue (`queueForOp`) |
| `label` | human-readable description shown in the UI |
| `step` | current stage, e.g. `embedding`, `prefilter (similarity)` |
| `queueTaskId` | asynq task id — the link to cancel, retry, and sweep |
| `refId` | the produced artefact's id on success |
| `error` | terminal reason |
| `meta` | free-form jsonb |

## Recorder API

```go
func New(ctx, q Store, op, label string, jobID, sourceKey *string, taskID string) *Recorder
func FromID(q Store, id string) *Recorder

func (r *Recorder) Start(ctx)
func (r *Recorder) Step(ctx, label string, meta map[string]any)
func (r *Recorder) Heartbeat(ctx)
func (r *Recorder) SetTimeout(ctx, ms int32)
func (r *Recorder) Ok(ctx, refID string, meta map[string]any)
func (r *Recorder) Fail(ctx, err error)
func (r *Recorder) Cancel(ctx, reason string)
func (r *Recorder) TimedOut(ctx, elapsed, limit time.Duration)
```

Two construction paths, for the two sides of the queue:

- **`New`** — the HTTP handler or scheduler creates the run *before* enqueueing, so a task
  is observable even while queued.
- **`FromID`** — the worker attaches to the existing run using the `activityId` from the
  payload.

`Recorder` is nil-safe (`valid()`, `recorder.go:40-42`): code paths without a run call the
same methods and they no-op.

```mermaid
sequenceDiagram
    participant H as Handler (HTTP)
    participant A as activity.New
    participant Q as asynq
    participant W as Worker
    participant F as activity.FromID
    H->>A: New(op, label, jobID, sourceKey, taskID)
    A-->>H: run id (state=queued)
    H->>Q: Enqueue(payload{activityId})
    Q->>W: deliver
    W->>F: FromID(activityId)
    F->>F: SetTimeout(policy.MaxDuration)
    F->>F: Start()
    loop while working
        F->>F: Step("...")
        F->>F: Heartbeat()
    end
    alt success
        F->>F: Ok(refID, meta)
    else error
        F->>F: Fail(err)
    else rate limited
        F->>F: Cancel(reason)
    else deadline
        F->>F: TimedOut(elapsed, limit)
    end
```

## States

```mermaid
stateDiagram-v2
    [*] --> queued: New
    queued --> running: Start
    running --> running: Step / Heartbeat
    running --> succeeded: Ok
    running --> failed: Fail
    running --> cancelled: Cancel
    running --> timed_out: TimedOut
    running --> interrupted: sweeper — heartbeat stale
    queued --> interrupted: sweeper — queued task gone
    succeeded --> [*]
    failed --> [*]
    cancelled --> [*]
    timed_out --> [*]
    interrupted --> [*]
```

## The sweeper

A worker killed mid-task cannot mark its own run. `activity.Sweeper` closes those out
(`sweeper.go:40-52`).

```go
func NewSweeper(store SweeperStore, inspector Inspector,
                staleAfter, sweepInterval, queuedGrace time.Duration) *Sweeper
```

It runs **once at startup** — catching runs orphaned by the previous process — then every
`ACTIVITY_SWEEP_INTERVAL` (`sweeper.go:65-77`).

### Two sweeps

```mermaid
flowchart TD
    S["sweepOnce"] --> R["sweepRunning"]
    S --> Q["sweepQueued"]
    R --> RC["cutoff = now - ACTIVITY_STALE_AFTER"]
    RC --> RQ["SweepStaleRunningActivityRuns"]
    RQ --> RM["mark interrupted: 'no worker heartbeat for at least X'"]
    Q --> QC["cutoff = now - ACTIVITY_QUEUED_GRACE"]
    QC --> QL["ListStaleQueuedActivityRuns"]
    QL --> QE{"asynq Inspector: task still exists?"}
    QE -->|yes| SKIP["leave it alone"]
    QE -->|no| QM["mark interrupted: 'queued task no longer exists'"]
```

The queued sweep asks Redis before concluding anything: `queuedTaskStillExists` maps
`row.Op` to a queue name via `queueForOp` and calls `Inspector.GetTaskInfo`
(`sweeper.go:121-130`). A long-queued but genuinely pending task is not touched.

:::note The sweeper never reopens a finished run
Its comment is explicit: the underlying queries filter to `running` / `queued`, *"so a run
that finished between sweep read and write is never re-opened."* This is why the fix is a
narrow query rather than a read-modify-write in Go.
:::

### Bounds, validated at startup

`validateLiveness` (`internal/queue/policy.go:113-125`) enforces:

| Rule | Reason |
| --- | --- |
| `ACTIVITY_HEARTBEAT_INTERVAL > 0` | there must be a heartbeat |
| `ACTIVITY_STALE_AFTER >= 2 × heartbeat` | one missed beat must not look like death |
| `ACTIVITY_STALE_AFTER + ACTIVITY_SWEEP_INTERVAL < 5m` | a dead run is visible within five minutes |

Defaults: heartbeat `30s`, stale after `2m`, sweep `1m`, queued grace `30m`.

## HTTP surface

| Method | Path | Effect |
| --- | --- | --- |
| GET | `/api/activity` | recent runs |
| GET | `/api/activity/queues` | per-queue backlog and the provider class each queue will use |
| POST | `/api/activity/retry` | retry failed or cancelled work |
| POST | `/api/activity/{id}/cancel` | cancel one run |
| POST | `/api/activity/cancel-all` | cancel everything queued |

`NewActivityHandler(queries, client, inspector, policies, resolvers)`
(`cmd/server/compose.go:52-57`) receives the four LLM routers as `ClassResolver`s, which is
how `/activity/queues` reports whether `match` is currently running hosted or local.

```mermaid
flowchart LR
    UI["Status page"] -->|poll| API["/api/activity"]
    API --> AR[("ActivityRun")]
    UI --> QAPI["/api/activity/queues"]
    QAPI --> INS["asynq Inspector"]
    QAPI --> RES["Routers: ProviderClass()"]
    UI -->|actions| ACT["retry / cancel / cancel-all"]
    ACT --> CLI["asynq Client + Inspector"]
```

## Reading a run

| You see | It means |
| --- | --- |
| `queued` for a long time | a worker slot is busy, or the provider class is saturated |
| `running` with an old `step` | the task is inside a long model call — check the heartbeat |
| `interrupted` | the process died, or the queued task vanished from Redis |
| `timed_out` | the task exceeded its `MaxDuration`; safe to retry |
| `cancelled` | a provider rate limit tripped the breaker |
| `failed` with a provider reason | terminal: key, credits, or model — fix and retry |
