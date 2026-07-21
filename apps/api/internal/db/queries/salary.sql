-- name: UpsertSalaryCache :exec
INSERT INTO "SalaryCache" (bucket, "salaryMin", "salaryMax", currency, source, "sampleSize")
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (bucket, currency, source) DO UPDATE SET
  "salaryMin" = EXCLUDED."salaryMin",
  "salaryMax" = EXCLUDED."salaryMax",
  "sampleSize" = "SalaryCache"."sampleSize" + 1,
  "updatedAt" = now();

-- name: GetSalaryCacheByBucket :many
SELECT * FROM "SalaryCache" WHERE bucket = $1;

-- name: UpdateJobSalary :exec
UPDATE "Job" SET
  "salaryMin" = $2,
  "salaryMax" = $3,
  "salaryCurrency" = $4,
  "salaryConfidence" = $5,
  "salarySource" = $6
WHERE "id" = $1;
