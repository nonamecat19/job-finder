-- name: ListJobsByScore :many
SELECT j.*, mr."id" AS mr_id, mr."similarity" AS mr_similarity, mr."score" AS mr_score,
  mr."matchedSkills" AS mr_matched_skills, mr."missingSkills" AS mr_missing_skills,
  mr."summary" AS mr_summary, mr."redFlags" AS mr_red_flags, mr."model" AS mr_model,
  mr."createdAt" AS mr_created_at
FROM "Job" j
LEFT JOIN "MatchResult" mr ON mr."jobId" = j."id"
WHERE (sqlc.narg('source')::text IS NULL OR j."sourceKey" = sqlc.narg('source'))
  AND (
    (sqlc.narg('status')::text IS NOT NULL AND j."status" = sqlc.narg('status'))
    OR (sqlc.narg('status')::text IS NULL AND j."status" != 'hidden')
  )
  AND (sqlc.narg('remote')::bool IS NULL OR j."remote" = sqlc.narg('remote'))
  AND (
    sqlc.narg('q')::text IS NULL
    OR j."title" ILIKE sqlc.narg('q')
    OR j."company" ILIKE sqlc.narg('q')
    OR j."description" ILIKE sqlc.narg('q')
  )
  AND (sqlc.narg('min_score')::int IS NULL OR mr."score" >= sqlc.narg('min_score'))
ORDER BY mr."score" DESC NULLS LAST, j."ingestedAt" DESC
OFFSET sqlc.arg('offset')
LIMIT sqlc.arg('limit');

-- name: ListJobsByDate :many
SELECT j.*, mr."id" AS mr_id, mr."similarity" AS mr_similarity, mr."score" AS mr_score,
  mr."matchedSkills" AS mr_matched_skills, mr."missingSkills" AS mr_missing_skills,
  mr."summary" AS mr_summary, mr."redFlags" AS mr_red_flags, mr."model" AS mr_model,
  mr."createdAt" AS mr_created_at
FROM "Job" j
LEFT JOIN "MatchResult" mr ON mr."jobId" = j."id"
WHERE (sqlc.narg('source')::text IS NULL OR j."sourceKey" = sqlc.narg('source'))
  AND (
    (sqlc.narg('status')::text IS NOT NULL AND j."status" = sqlc.narg('status'))
    OR (sqlc.narg('status')::text IS NULL AND j."status" != 'hidden')
  )
  AND (sqlc.narg('remote')::bool IS NULL OR j."remote" = sqlc.narg('remote'))
  AND (
    sqlc.narg('q')::text IS NULL
    OR j."title" ILIKE sqlc.narg('q')
    OR j."company" ILIKE sqlc.narg('q')
    OR j."description" ILIKE sqlc.narg('q')
  )
  AND (sqlc.narg('min_score')::int IS NULL OR mr."score" >= sqlc.narg('min_score'))
ORDER BY j."ingestedAt" DESC
OFFSET sqlc.arg('offset')
LIMIT sqlc.arg('limit');

-- name: CountJobs :one
SELECT count(*)
FROM "Job" j
LEFT JOIN "MatchResult" mr ON mr."jobId" = j."id"
WHERE (sqlc.narg('source')::text IS NULL OR j."sourceKey" = sqlc.narg('source'))
  AND (
    (sqlc.narg('status')::text IS NOT NULL AND j."status" = sqlc.narg('status'))
    OR (sqlc.narg('status')::text IS NULL AND j."status" != 'hidden')
  )
  AND (sqlc.narg('remote')::bool IS NULL OR j."remote" = sqlc.narg('remote'))
  AND (
    sqlc.narg('q')::text IS NULL
    OR j."title" ILIKE sqlc.narg('q')
    OR j."company" ILIKE sqlc.narg('q')
    OR j."description" ILIKE sqlc.narg('q')
  )
  AND (sqlc.narg('min_score')::int IS NULL OR mr."score" >= sqlc.narg('min_score'));

-- name: GetJobDocuments :many
SELECT * FROM "GeneratedDocument" WHERE "jobId" = $1 ORDER BY "createdAt" DESC;
