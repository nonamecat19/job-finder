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
UPDATE "Job" SET "embedding" = $2, "embedModel" = $3 WHERE "id" = $1;

-- name: UpdateJobEmbeddingWithHash :exec
-- Stores the hash of the exact text embedded, so a later match on unchanged
-- content can skip re-embedding (019-ai-job-throughput, research.md R5).
UPDATE "Job" SET "embedding" = $2, "embeddingHash" = $3, "embedModel" = $4 WHERE "id" = $1;

-- name: ClearStaleJobEmbeddings :exec
UPDATE "Job" SET "embedding" = NULL, "embeddingHash" = NULL WHERE "embedModel" IS DISTINCT FROM $1;

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
  "company" = COALESCE(NULLIF(sqlc.arg('company'), ''), "company"),
  "description" = COALESCE(NULLIF(sqlc.arg('description'), ''), "description"),
  "salaryRaw" = COALESCE(sqlc.narg('salaryRaw'), "salaryRaw"),
  "location" = COALESCE(sqlc.narg('location'), "location"),
  "remote" = "remote" OR sqlc.arg('remote')::bool,
  "raw" = sqlc.arg('raw'),
  "postedAt" = COALESCE(sqlc.narg('postedAt'), "postedAt"),
  "detailScrapedAt" = now(),
  "experience_level" = COALESCE(sqlc.narg('experience_level'), "experience_level"),
  "experience_min_years" = COALESCE(sqlc.narg('experience_min_years'), "experience_min_years"),
  "english_level" = COALESCE(sqlc.narg('english_level'), "english_level"),
  "salary_estimate_raw" = COALESCE(sqlc.narg('salary_estimate_raw'), "salary_estimate_raw"),
  "salary_estimate_min" = COALESCE(sqlc.narg('salary_estimate_min'), "salary_estimate_min"),
  "salary_estimate_max" = COALESCE(sqlc.narg('salary_estimate_max'), "salary_estimate_max"),
  "salary_estimate_currency" = COALESCE(sqlc.narg('salary_estimate_currency'), "salary_estimate_currency")
WHERE "id" = sqlc.arg('id')
RETURNING *;

-- name: ClearJobDetailScrapedAt :exec
UPDATE "Job" SET "detailScrapedAt" = NULL WHERE "id" = $1;

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

-- name: GetJobsByDedupeKeys :many
-- Batch replacement for the per-posting GetJobByDedupeKey in the ingest
-- persist loop. Returns only the subset of $1 that already exists, so the
-- caller classifies known vs new from one round trip.
SELECT "id", "dedupeKey" FROM "Job" WHERE "dedupeKey" = ANY(sqlc.arg('dedupe_keys')::text[]);

-- name: FindJobsByCompanies :many
-- Batch replacement for FindJobByCompany. One candidate per company: the most
-- recently ingested job from a DIFFERENT source, matching the per-posting
-- query's ORDER BY "ingestedAt" DESC LIMIT 1 semantics.
-- Served by "Job_lower_company_idx" (migration 00032) — the per-posting form
-- had no supporting index and scanned the whole table for every board posting.
SELECT DISTINCT ON (LOWER("company"))
  LOWER("company") AS company_key, "id", "sourceKey", "title"
FROM "Job"
WHERE LOWER("company") = ANY(sqlc.arg('companies')::text[])
  AND "sourceKey" != sqlc.arg('source_key')
ORDER BY LOWER("company"), "ingestedAt" DESC;

-- name: BulkInsertJobs :many
-- One statement per chunk, replacing N InsertJob calls. ON CONFLICT DO NOTHING
-- handles both in-batch duplicates that survived Go-side dedup and genuine
-- races with a concurrent run: the loser gets no RETURNING row, so it queues
-- no downstream work and neither run fails.
-- COPY was rejected here precisely because it cannot express ON CONFLICT.
-- "postedAt" is cast to timestamp[], not timestamptz[]: the column is
-- timestamp(3) and the rest of the ingest path carries pgtype.Timestamp.
-- The arrays are zipped by parallel unnest() calls in a subquery select list
-- rather than the multi-argument unnest(a, b, ...) table form, which sqlc's
-- analyzer cannot resolve. PostgreSQL evaluates several set-returning
-- functions in one select list in lockstep (ROWS FROM semantics), so the
-- position alignment is identical.
INSERT INTO "Job" (
  "dedupeKey", "sourceKey", "externalId", "title", "company", "location",
  "remote", "salaryRaw", "url", "description", "raw", "postedAt",
  "subscriptionId", "seenOnSources"
)
SELECT
  d, s, NULLIF(e, ''), t, c, NULLIF(l, ''), r, NULLIF(sr, ''), u, ds, rw, pa,
  sqlc.arg('subscription_id'), ARRAY[s]
FROM (
  SELECT
    unnest(sqlc.arg('dedupe_keys')::text[]) AS d,
    unnest(sqlc.arg('source_keys')::text[]) AS s,
    unnest(sqlc.arg('external_ids')::text[]) AS e,
    unnest(sqlc.arg('titles')::text[]) AS t,
    unnest(sqlc.arg('companies')::text[]) AS c,
    unnest(sqlc.arg('locations')::text[]) AS l,
    unnest(sqlc.arg('remotes')::bool[]) AS r,
    unnest(sqlc.arg('salary_raws')::text[]) AS sr,
    unnest(sqlc.arg('urls')::text[]) AS u,
    unnest(sqlc.arg('descriptions')::text[]) AS ds,
    unnest(sqlc.arg('raws')::jsonb[]) AS rw,
    unnest(sqlc.arg('posted_ats')::timestamp[]) AS pa
) AS x
ON CONFLICT ("dedupeKey") DO NOTHING
RETURNING "id", "dedupeKey";

-- name: BulkRecordJobReposts :execrows
-- Set-based replacement for N RecordJobRepost calls, plus the retry guard.
-- "lastSeenRunId" IS DISTINCT FROM $3 (not != $3) because the column is NULL
-- for every row predating migration 00032, and NULL != $3 is NULL, which would
-- exclude every pre-existing row from ever being counted.
UPDATE "Job" SET
  "seenCount" = "seenCount" + 1,
  "ingestedAt" = now(),
  "subscriptionId" = COALESCE("subscriptionId", sqlc.arg('subscription_id')),
  "lastSeenRunId" = sqlc.arg('run_id')
WHERE "dedupeKey" = ANY(sqlc.arg('dedupe_keys')::text[])
  AND "lastSeenRunId" IS DISTINCT FROM sqlc.arg('run_id');

-- name: BulkMergeJobBoards :execrows
-- Batch form of MergeJobBoard: folds board-vendor postings into an existing
-- job from another source. Arrays are position-aligned; unnest pairs them.
UPDATE "Job" SET
  "url" = x.url,
  "sourceKey" = x.source_key,
  "seenOnSources" = array_append("seenOnSources", x.source_key)
FROM (
  SELECT unnest(sqlc.arg('ids')::uuid[]) AS id,
         unnest(sqlc.arg('urls')::text[]) AS url,
         unnest(sqlc.arg('source_keys')::text[]) AS source_key
) AS x
WHERE "Job"."id" = x.id;
