-- name: GetKeywordDiffByJobID :one
SELECT * FROM "KeywordDiff" WHERE "jobId" = $1;

-- name: UpsertKeywordDiff :one
INSERT INTO "KeywordDiff" (
  "jobId", "matched", "missingRequired", "missingPreferred", "coveragePct", "model"
) VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT ("jobId") DO UPDATE SET
  "matched" = EXCLUDED."matched",
  "missingRequired" = EXCLUDED."missingRequired",
  "missingPreferred" = EXCLUDED."missingPreferred",
  "coveragePct" = EXCLUDED."coveragePct",
  "model" = EXCLUDED."model"
RETURNING *;

-- name: DeleteKeywordDiffByJobID :exec
DELETE FROM "KeywordDiff" WHERE "jobId" = $1;
