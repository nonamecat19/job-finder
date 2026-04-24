-- name: GetApplicationByID :one
SELECT * FROM "Application" WHERE "id" = $1;

-- name: GetApplicationByJobID :one
SELECT * FROM "Application" WHERE "jobId" = $1;

-- name: UpsertApplicationStatus :exec
INSERT INTO "Application" ("jobId", "status", "events")
VALUES ($1, $2, $3)
ON CONFLICT ("jobId") DO UPDATE SET "status" = EXCLUDED."status";

-- name: UpdateApplication :one
UPDATE "Application" SET
  "status" = COALESCE(sqlc.narg('status'), "status"),
  "notes" = CASE WHEN sqlc.narg('notes_set')::bool THEN sqlc.narg('notes') ELSE "notes" END,
  "events" = COALESCE(sqlc.narg('events'), "events"),
  "appliedAt" = COALESCE(sqlc.narg('applied_at'), "appliedAt"),
  "updatedAt" = now()
WHERE "id" = sqlc.arg('id')
RETURNING *;

-- name: ListApplications :many
SELECT
  a.*,
  j."id" AS job_id_full, j."dedupeKey" AS job_dedupe_key, j."sourceKey" AS job_source_key,
  j."title" AS job_title, j."company" AS job_company, j."location" AS job_location,
  j."remote" AS job_remote, j."salaryRaw" AS job_salary_raw, j."url" AS job_url,
  j."description" AS job_description, j."postedAt" AS job_posted_at, j."ingestedAt" AS job_ingested_at,
  j."status" AS job_status,
  mr."id" AS mr_id, mr."similarity" AS mr_similarity, mr."score" AS mr_score,
  mr."matchedSkills" AS mr_matched_skills, mr."missingSkills" AS mr_missing_skills,
  mr."summary" AS mr_summary, mr."redFlags" AS mr_red_flags, mr."model" AS mr_model,
  mr."createdAt" AS mr_created_at
FROM "Application" a
JOIN "Job" j ON j."id" = a."jobId"
LEFT JOIN "MatchResult" mr ON mr."jobId" = j."id"
WHERE (sqlc.narg('status')::text IS NULL OR a."status" = sqlc.narg('status'))
ORDER BY a."updatedAt" DESC;

-- name: StatsJobsTotal :one
SELECT count(*) FROM "Job" WHERE "status" != 'hidden';

-- name: StatsJobsLast24h :one
SELECT count(*) FROM "Job" WHERE "ingestedAt" >= $1;

-- name: StatsHighFit :one
SELECT count(*) FROM "MatchResult" WHERE "score" >= 70;

-- name: StatsPipeline :many
SELECT "status", count(*) AS count FROM "Application" GROUP BY "status" ORDER BY "status" ASC;
