-- name: UpsertJobSignal :one
-- FR-009: at most one result per (job, kind); a re-score replaces the row
-- rather than accumulating history.
INSERT INTO "JobSignal" ("jobId", "kind", "score", "signals", "model")
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT ("jobId", "kind") DO UPDATE SET
  "score" = EXCLUDED."score",
  "signals" = EXCLUDED."signals",
  "model" = EXCLUDED."model",
  "createdAt" = now()
RETURNING *;

-- name: GetJobSignal :one
SELECT * FROM "JobSignal" WHERE "jobId" = $1 AND "kind" = $2;

-- name: ListJobSignalsByJobIds :many
-- Batch fetch for the feed: one query for the whole page, not one per job.
SELECT * FROM "JobSignal"
WHERE "jobId" = ANY(sqlc.arg('job_ids')::uuid[]) AND "kind" = sqlc.arg('kind');

-- name: CountRepostsByDedupeKey :one
-- Repost signal (FR-002a / FR-003). Reads the durable "seenCount" column
-- (see migration 00016), NOT a recomputed identity or a naive
-- `count(*) ... GROUP BY "dedupeKey"` — the latter can only ever return 1
-- because "dedupeKey" carries Job_dedupeKey_unique. "seenCount" is bumped by
-- ingestion.persistIfNew every time the same dedupeKey is re-ingested, so
-- this genuinely reflects "how many times has this posting reappeared".
SELECT "seenCount"::int FROM "Job" WHERE "dedupeKey" = $1;

-- name: ListJobsForCrossBoardCheck :many
-- Candidate rows for the cross-board-duplicate signal (FR-002c / FR-005).
-- Postgres has no fuzzy-text-similarity primitive available here, so this
-- query supplies candidates within the 60-day window and
-- ghostjob.MeasureSignals does the simhash comparison + counts *distinct*
-- "sourceKey" values among near-duplicates app-side. Bounded by LIMIT for
-- single-user scale (assumption: thousands of rows, not millions).
SELECT "id", "sourceKey", "description"
FROM "Job"
WHERE "id" != $1
  AND "ingestedAt" >= now() - interval '60 days'
ORDER BY "ingestedAt" DESC
LIMIT 500;

-- name: CountAlwaysHiringByCompany :one
-- Always-hiring signal (FR-002d / FR-005 / FR-006), 90-day window. Counts
-- this company's postings that never progressed past initial discovery: the
-- fast-path "status = 'found'" is narrowed by an authoritative check against
-- "Application" (rejected counts as progression, not as unprogressed — a
-- rejection means a real process happened). Includes the job's own row: a
-- lone posting yields 1, which the service/prompt treats as no evidence
-- (SC-004), never as a signal skip.
SELECT count(*)::int
FROM "Job" j
WHERE lower(j."company") = lower($1)
  AND j."ingestedAt" >= now() - interval '90 days'
  AND j."status" = 'found'
  AND NOT EXISTS (
    SELECT 1 FROM "Application" a
    WHERE a."jobId" = j."id"
      AND a."status" != 'found'
  );
