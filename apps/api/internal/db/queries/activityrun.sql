-- name: InsertActivityRun :one
INSERT INTO "ActivityRun" ("op", "label", "jobId", "sourceKey", "queueTaskId", "meta")
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: StartActivityRun :exec
UPDATE "ActivityRun" SET "state" = 'running', "startedAt" = now(), "heartbeatAt" = now() WHERE "id" = $1;

-- name: SetActivityStep :exec
UPDATE "ActivityRun" SET "step" = $2, "meta" = COALESCE(sqlc.narg('meta'), "meta"), "heartbeatAt" = now() WHERE "id" = $1;

-- name: TouchActivityRunHeartbeat :exec
UPDATE "ActivityRun" SET "heartbeatAt" = now() WHERE "id" = $1 AND "state" = 'running';

-- name: SetActivityRunTimeout :exec
-- Records the deadline (from the task-type policy) the run was admitted
-- under, so the sweeper and UI can report which limit applied. Set by the
-- queue middleware, independent of the handler's own StartActivityRun call.
UPDATE "ActivityRun" SET "timeoutMs" = $2 WHERE "id" = $1;

-- name: FinishActivityRunOk :exec
UPDATE "ActivityRun" SET "state" = 'succeeded', "refId" = $2, "meta" = COALESCE(sqlc.narg('meta'), "meta"), "finishedAt" = now() WHERE "id" = $1;

-- name: FinishActivityRunError :exec
UPDATE "ActivityRun" SET "state" = 'failed', "error" = $2, "finishedAt" = now() WHERE "id" = $1;

-- name: FinishActivityRunCancelled :exec
UPDATE "ActivityRun" SET "state" = 'cancelled', "error" = $2, "finishedAt" = now() WHERE "id" = $1;

-- name: FinishActivityRunTimedOut :exec
UPDATE "ActivityRun" SET "state" = 'timed_out', "error" = $2, "finishedAt" = now() WHERE "id" = $1 AND "state" = 'running';

-- name: SweepStaleRunningActivityRuns :many
-- Stale: heartbeatAt older than sqlc.arg('cutoff'), or null (the existing-ghost
-- case — pre-migration rows and any queued->running transition that predates
-- this feature). Only ever moves 'running' rows, never a terminal one, so a
-- run that finished between sweep read and write is never re-opened.
UPDATE "ActivityRun"
SET "state" = 'interrupted', "error" = sqlc.arg('reason'), "finishedAt" = now()
WHERE "state" = 'running' AND ("heartbeatAt" IS NULL OR "heartbeatAt" < sqlc.arg('cutoff'))
RETURNING *;

-- name: ListStaleQueuedActivityRuns :many
-- Rows past the queued grace window (ACTIVITY_QUEUED_GRACE). RabbitMQ has
-- no per-message "does this still exist" query the way asynq's Inspector
-- did, so these rows are DB-authoritative: past the grace window means
-- interrupted, unconditionally.
SELECT * FROM "ActivityRun" WHERE "state" = 'queued' AND "createdAt" < sqlc.arg('cutoff');

-- name: FinishActivityRunInterrupted :exec
-- Per-row finalizer for a stale queued run past ACTIVITY_QUEUED_GRACE, one
-- row at a time.
UPDATE "ActivityRun" SET "state" = 'interrupted', "error" = $2, "finishedAt" = now() WHERE "id" = $1 AND "state" = 'queued';

-- name: GetActivityRun :one
SELECT * FROM "ActivityRun" WHERE "id" = $1;

-- name: ListActiveActivityRuns :many
SELECT * FROM "ActivityRun" WHERE "state" IN ('queued', 'running') ORDER BY "createdAt" DESC;

-- name: ListRecentActivityRuns :many
SELECT * FROM "ActivityRun" ORDER BY "createdAt" DESC LIMIT $1;

-- name: ListFailedActivityRuns :many
-- Includes "cancelled" (e.g. skipped because of an upstream rate limit),
-- "timed_out" (exceeded its task-type deadline), and "interrupted" (worker
-- vanished) alongside "failed" — all four are retryable the same way.
SELECT * FROM "ActivityRun"
WHERE "state" IN ('failed', 'cancelled', 'timed_out', 'interrupted') AND (sqlc.narg('op')::text IS NULL OR "op" = sqlc.narg('op'))
ORDER BY "createdAt" DESC;

-- name: DeleteActivityRunsBefore :exec
DELETE FROM "ActivityRun" WHERE "createdAt" < $1;

-- name: BulkInsertActivities :many
-- Batch form of the per-task activity.New insert. Without this the N+1 simply
-- moves from the persist loop to the enqueue loop and SC-002 is not met.
-- Returns "jobId" and "op" alongside "id" so the caller correlates by
-- (job, op) rather than by row order, which is not guaranteed.
-- The column set follows the actual "ActivityRun" schema (00004/00030), not
-- the kind/status names in the contract: the table's columns are "op" and
-- "state", "state" defaults to 'queued' and "meta" defaults to '{}'.
-- The arrays are zipped by parallel unnest() calls in a subquery select list
-- rather than the multi-argument unnest(a, b, ...) table form, which sqlc's
-- analyzer cannot resolve; PostgreSQL evaluates them in lockstep either way.
INSERT INTO "ActivityRun" ("op", "label", "jobId", "sourceKey")
SELECT o, l, j, NULLIF(sk, '')
FROM (
  SELECT unnest(sqlc.arg('ops')::text[]) AS o,
         unnest(sqlc.arg('labels')::text[]) AS l,
         unnest(sqlc.arg('job_ids')::uuid[]) AS j,
         unnest(sqlc.arg('source_keys')::text[]) AS sk
) AS x
RETURNING "id", "jobId", "op";
