# Data Model: Asynqmon Queue Monitoring Dashboard

No new persisted data model is introduced by this feature. asynqmon reads/writes the
task-queue state that the existing asynq workers already maintain in Redis
(`docker-compose.yml` → `redis` service). The entities below are the existing runtime
concepts the dashboard surfaces and manipulates — nothing new is stored, and no schema
migration is required.

## Queue

Represents one of the six existing named asynq queues. Already defined by
`apps/api/internal/queue` and wired in `apps/api/cmd/server/servers.go`.

| Field | Description |
|---|---|
| name | One of: `ingest`, `match`, `generate`, `enrich`, `salary`, `ghost` |
| pending count | Tasks queued, not yet picked up |
| active count | Tasks currently being processed |
| scheduled count | Tasks deferred to run later |
| retry count | Tasks that failed and are queued for retry |
| archived count | Tasks that exhausted retries or were manually archived |
| completed count | Tasks that finished successfully (within retention window) |
| daily history | Recent per-day processed/failed counts (FR-005) |

## Task

Represents one unit of work within a queue. Already defined by asynq's internal task
representation (payload built by each producer, e.g. `apps/api/internal/ingestion`).

| Field | Description |
|---|---|
| id | asynq-assigned task identifier |
| type | Task type string (e.g. `queue.TypeIngest`, `queue.TypeMatch`, ...) |
| queue | Which of the six queues it belongs to |
| payload | Task-specific data (job/source/document identifiers, etc.) |
| state | pending / active / scheduled / retry / archived / completed |
| retry count | Number of attempts made so far |
| last error | Failure message, present when state is retry/archived |

## Relationships

- A **Task** belongs to exactly one **Queue** (1:N).
- No relationship to job-finder's own domain entities (Job, Application, etc.) is
  modeled here — the dashboard shows queue/task metadata only, not the underlying
  business records those tasks operate on (per spec Assumptions: "not application/
  business data such as jobs or applications").
