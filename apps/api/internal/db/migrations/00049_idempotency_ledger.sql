-- +goose Up
-- The dedupe ledger for result events (data-model.md § 7). Written in the
-- same transaction as the result it admits: the primary key on
-- idempotency_key is what makes a redelivered result event a no-op insert
-- rather than a duplicate stored result (FR-030). run_id lets a later
-- superseded attempt (a retry whose earlier attempt's result arrives after a
-- newer one already landed) be recognised and discarded rather than
-- overwriting the accepted result (FR-037).
--
-- Short-horizon dedupe window, not an audit log: pruned by the activity
-- sweeper once older than the longest retry budget plus margin (T031).
CREATE TABLE idempotency_ledger (
    idempotency_key text PRIMARY KEY,
    work_type text NOT NULL,
    run_id uuid NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idempotency_ledger_accepted_at_idx ON idempotency_ledger (accepted_at);

-- +goose Down
DROP TABLE idempotency_ledger;
