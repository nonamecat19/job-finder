-- +goose Up
ALTER TABLE host_retrieval_state DROP COLUMN budget_period_start;
ALTER TABLE host_retrieval_state DROP COLUMN budget_used;
ALTER TABLE host_retrieval_state DROP COLUMN budget_limit;

-- +goose Down
ALTER TABLE host_retrieval_state ADD COLUMN budget_period_start TIMESTAMPTZ;
ALTER TABLE host_retrieval_state ADD COLUMN budget_used INTEGER NOT NULL DEFAULT 0;
ALTER TABLE host_retrieval_state ADD COLUMN budget_limit INTEGER NOT NULL DEFAULT 200;
