-- name: InsertIdempotencyLedgerEntry :one
-- The dedupe write (data-model.md § 7): a redelivery of the same
-- idempotency_key hits the primary key and inserts nothing, which is how the
-- caller (events.Admit) tells a fresh accept from a duplicate/superseded one.
INSERT INTO idempotency_ledger (idempotency_key, work_type, run_id)
VALUES ($1, $2, $3)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: GetIdempotencyLedgerEntry :one
SELECT * FROM idempotency_ledger WHERE idempotency_key = $1;

-- name: PruneIdempotencyLedger :execrows
-- Retention: rows older than the longest retry budget plus a margin
-- (data-model.md § 7). Cutoff is computed by the caller.
DELETE FROM idempotency_ledger WHERE accepted_at < $1;
