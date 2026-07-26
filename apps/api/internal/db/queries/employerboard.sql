-- name: ListEmployerBoards :many
SELECT * FROM "EmployerBoard" ORDER BY "vendor", "displayName";

-- name: ListEmployerBoardsByVendor :many
SELECT * FROM "EmployerBoard" WHERE "vendor" = $1 AND "enabled" = true ORDER BY "displayName";

-- name: GetEmployerBoard :one
SELECT * FROM "EmployerBoard" WHERE "vendor" = $1 AND "employerIdentifier" = $2;

-- name: InsertEmployerBoard :one
INSERT INTO "EmployerBoard" ("vendor", "employerIdentifier", "displayName", "addedVia")
VALUES ($1, $2, $3, $4)
ON CONFLICT ("vendor", "employerIdentifier") DO UPDATE SET "enabled" = true
RETURNING *;

-- name: DeleteEmployerBoard :exec
DELETE FROM "EmployerBoard" WHERE "id" = $1;

-- name: RecordEmployerBoardRunOutcome :exec
-- postingCount > 0 resets the stale counter; 0 increments it (FR-014).
UPDATE "EmployerBoard" SET
  "lastSuccessAt" = now(),
  "lastPostingCount" = $2,
  "consecutiveEmptyRuns" = CASE WHEN $2::int > 0 THEN 0 ELSE "consecutiveEmptyRuns" + 1 END
WHERE "id" = $1;

-- name: ListBoardCandidates :many
SELECT * FROM "BoardCandidate" WHERE "state" = 'proposed' ORDER BY "createdAt" DESC;

-- name: GetBoardCandidate :one
SELECT * FROM "BoardCandidate" WHERE "vendor" = $1 AND "employerIdentifier" = $2;

-- name: GetBoardCandidateByID :one
SELECT * FROM "BoardCandidate" WHERE "id" = $1;

-- name: InsertBoardCandidate :one
INSERT INTO "BoardCandidate" ("vendor", "employerIdentifier", "inferredFromJobId")
VALUES ($1, $2, $3)
ON CONFLICT ("vendor", "employerIdentifier") DO NOTHING
RETURNING *;

-- name: DecideBoardCandidate :exec
UPDATE "BoardCandidate" SET "state" = $2, "decidedAt" = now() WHERE "id" = $1;

-- name: ListApplyURLsForDiscovery :many
-- Every distinct job URL, newest first, scanned by candidate discovery for
-- supported-vendor links (research.md §3). Bounded so discovery over a large
-- Job table stays cheap; re-running periodically still catches older links
-- eventually via the LIMIT window sliding as new jobs are ingested above it.
SELECT DISTINCT "url" FROM "Job" ORDER BY "ingestedAt" DESC LIMIT $1;
