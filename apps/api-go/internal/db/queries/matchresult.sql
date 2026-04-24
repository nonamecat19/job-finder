-- name: GetMatchResultByJobID :one
SELECT * FROM "MatchResult" WHERE "jobId" = $1;

-- name: UpsertMatchResult :one
INSERT INTO "MatchResult" (
  "jobId", "similarity", "score", "matchedSkills", "missingSkills", "summary", "redFlags", "model"
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT ("jobId") DO UPDATE SET
  "similarity" = EXCLUDED."similarity",
  "score" = EXCLUDED."score",
  "matchedSkills" = EXCLUDED."matchedSkills",
  "missingSkills" = EXCLUDED."missingSkills",
  "summary" = EXCLUDED."summary",
  "redFlags" = EXCLUDED."redFlags",
  "model" = EXCLUDED."model"
RETURNING *;
