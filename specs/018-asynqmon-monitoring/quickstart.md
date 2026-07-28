# Quickstart: Asynqmon Queue Monitoring Dashboard

Validates the feature end-to-end: dashboard visibility (User Story 1) and task
retry/delete actions (User Story 2). See [data-model.md](./data-model.md) for the
Queue/Task fields referenced below.

## Prerequisites

- Docker Compose stack configured as usual (`.env` with `DB_PASSWORD` set).
- No extra setup beyond the normal dev workflow (FR-007).

## Setup

```bash
make run-all
# or: docker compose up
```

This brings up `postgres`, `redis`, `ollama`, `minio`, `createbuckets`, and the new
`asynqmon` service together — no separate command needed.

## Validate: dashboard shows live queue state (User Story 1)

1. Trigger a background task, e.g. run an ingestion cycle (existing app flow that
   enqueues onto the `ingest` queue).
2. Open `http://localhost:8090` in a browser.
3. **Expected**: all six queues (`ingest`, `match`, `generate`, `enrich`, `salary`,
   `ghost`) are listed with pending/active/scheduled/retry/archived/completed counts
   (FR-002). The `ingest` queue's active/pending count reflects the task just triggered.
4. Click into the `ingest` queue's task list. **Expected**: the task appears with its
   type and payload (FR-003).

## Validate: inspect and act on a failed task (User Story 2)

1. Force a task to fail — e.g. temporarily stop `ollama` and trigger a `generate` task
   so it errors, or seed a task type unhandled by any worker (whichever is quickest
   given current app state).
2. Wait for the task to exhaust retries and land in the queue's **archived** (or
   **retry**) list.
3. Open the task in the dashboard. **Expected**: last error message, retry count, and
   payload are visible (FR-003).
4. Click **Retry**. **Expected**: task moves back to pending/scheduled state and the
   queue counts update (FR-004).
5. Repeat on a different task and click **Delete** instead. **Expected**: task is
   removed and no longer counted in any queue total (FR-004).

## Validate: history view (User Story 3, best-effort)

1. After a mix of successful/failed tasks have run, open a queue's history tab.
2. **Expected**: per-day processed/failed counts are shown for a recent window (FR-005).

## Validate: dev-only exposure (FR-008)

```bash
grep -c asynqmon docker-compose.prod.yml   # expected: 0 (service must not appear)
docker compose config >/dev/null           # expected: exits 0, dev compose file is valid
```

## Validate: unreachable Redis handled gracefully (Edge Case)

1. `docker compose stop redis`
2. Reload `http://localhost:8090`. **Actual** (verified): the page does not crash or
   go blank — shell/nav/charts still render, tables show empty — but no visible error
   banner appears; the failure is only logged to the browser console
   (`listQueuesAsync: error: no error response data available`). This is stock
   asynqmon behavior with the unmodified upstream image (see tasks.md T013); tightening
   it would require forking the UI, out of scope for this feature.
3. `docker compose start redis` to restore normal state — the dashboard recovers
   automatically on next poll, no restart needed.
