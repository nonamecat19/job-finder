-- name: GetJobByID :one
SELECT * FROM "Job" WHERE "id" = $1;

-- name: GetJobByDedupeKey :one
SELECT "id" FROM "Job" WHERE "dedupeKey" = $1;

-- name: InsertJob :one
INSERT INTO "Job" (
  "dedupeKey", "sourceKey", "externalId", "title", "company", "location",
  "remote", "salaryRaw", "url", "description", "raw", "postedAt", "subscriptionId",
  "seenOnSources"
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
  ARRAY[$2]
)
RETURNING *;

-- name: UpdateJobEmbedding :exec
UPDATE "Job" SET "embedding" = $2 WHERE "id" = $1;

-- name: UpdateJobEmbeddingWithHash :exec
-- Stores the hash of the exact text embedded, so a later match on unchanged
-- content can skip re-embedding (019-ai-job-throughput, research.md R5).
UPDATE "Job" SET "embedding" = $2, "embeddingHash" = $3 WHERE "id" = $1;

-- name: UpdateJobStatus :one
UPDATE "Job" SET "status" = $2 WHERE "id" = $1
RETURNING *;

-- name: DeleteAllJobs :execrows
-- Wipes every job. FK "ON DELETE cascade" clears the dependent
-- Application / GeneratedDocument / MatchResult / Activity rows too.
DELETE FROM "Job";

-- name: ListJobsNeedingDetail :many
SELECT * FROM "Job"
WHERE "sourceKey" = $1 AND "detailScrapedAt" IS NULL
ORDER BY "ingestedAt" ASC
LIMIT $2;

-- name: RecordJobRepost :one
-- Called by ingestion.persistIfNew when a job with this dedupeKey already
-- exists: bumps "seenCount" and refreshes "ingestedAt" so the posting's
-- reappearance is durable, feeding the ghost-job repost signal (005).
-- Backfills "subscriptionId" when the job wasn't already attributed to one,
-- so a job first seen by an unrelated run still surfaces under a subscription
-- that later rediscovers it (dashboard "filter by Subscription").
UPDATE "Job" SET "seenCount" = "seenCount" + 1, "ingestedAt" = now(),
  "subscriptionId" = COALESCE("subscriptionId", $2)
WHERE "dedupeKey" = $1
RETURNING *;

-- name: UpdateJobDetail :one
UPDATE "Job" SET
  "description" = COALESCE(NULLIF(sqlc.arg('description'), ''), "description"),
  "salaryRaw" = COALESCE(sqlc.narg('salaryRaw'), "salaryRaw"),
  "location" = COALESCE(sqlc.narg('location'), "location"),
  "remote" = "remote" OR sqlc.arg('remote')::bool,
  "raw" = sqlc.arg('raw'),
  "postedAt" = COALESCE(sqlc.narg('postedAt'), "postedAt"),
  "detailScrapedAt" = now()
WHERE "id" = sqlc.arg('id')
RETURNING *;

-- name: FindJobByCompany :one
SELECT id, "sourceKey", "title" FROM "Job"
WHERE LOWER("company") = LOWER($1)
  AND "sourceKey" != $2
ORDER BY "ingestedAt" DESC
LIMIT 1;

-- name: MergeJobBoard :one
UPDATE "Job" SET
  "url" = $2,
  "sourceKey" = $3,
  "seenOnSources" = array_append("seenOnSources", $4)
WHERE "id" = $1
RETURNING *;

-- name: ListJobsMissingMatch :many
-- Jobs that were inserted but never produced a MatchResult row. Insert and
-- enqueue aren't atomic, so a crash (or a Redis blip) between the two strands
-- the job: no score, and therefore invisible in the score-sorted feed, with
-- nothing to ever retry it. The scheduler re-enqueues these.
--
-- Note a prefiltered-out job is NOT missing a match: matching writes a row
-- carrying just the similarity, so the sweep passes over it.
--
-- Bounded on both sides. "older_than" leaves in-flight jobs alone (including
-- ones still queued behind enrichment); "newer_than" stops a job whose
-- matching fails deterministically from being retried on every tick forever.
SELECT j."id", j."company", j."title" FROM "Job" j
LEFT JOIN "MatchResult" mr ON mr."jobId" = j."id"
WHERE mr."id" IS NULL
  AND j."status" != 'hidden'
  AND j."ingestedAt" < sqlc.arg('older_than')::timestamp
  AND j."ingestedAt" > sqlc.arg('newer_than')::timestamp
ORDER BY j."ingestedAt" DESC
LIMIT sqlc.arg('limit');
