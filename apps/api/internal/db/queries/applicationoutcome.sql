-- name: InsertApplicationOutcome :one
-- Append one outcome event. Idempotent for the terminal-once types
-- ('applied', 'offer', 'rejected') via the partial unique index — a redundant
-- transition inserts nothing and returns no row (pgx.ErrNoRows), which callers
-- treat as "already recorded". 'viewed'/'screen' are outside the index
-- predicate and always insert.
INSERT INTO "ApplicationOutcome" ("applicationId", "eventType", "occurredAt", "note")
VALUES ($1, $2, $3, $4)
ON CONFLICT ("applicationId", "eventType")
	WHERE "eventType" IN ('applied', 'offer', 'rejected')
	DO NOTHING
RETURNING *;

-- name: ListApplicationOutcomes :many
-- Timeline read: oldest first by real-world event time, "recordedAt" breaking
-- ties so two events back-dated to the same instant still return in write order.
SELECT * FROM "ApplicationOutcome"
WHERE "applicationId" = $1
ORDER BY "occurredAt" ASC, "recordedAt" ASC;

-- name: CountApplicationOutcomes :one
SELECT count(*) FROM "ApplicationOutcome" WHERE "applicationId" = $1;
