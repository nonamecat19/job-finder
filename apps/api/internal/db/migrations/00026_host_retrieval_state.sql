-- +goose Up
CREATE TABLE host_retrieval_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host TEXT NOT NULL UNIQUE,
    identity_version TEXT NOT NULL DEFAULT '',
    current_rung TEXT NOT NULL DEFAULT 'direct',
    rung_last_verified_at TIMESTAMPTZ,
    cookies JSONB,
    consecutive_blocks INTEGER NOT NULL DEFAULT 0,
    cooling_off_until TIMESTAMPTZ,
    last_block_at TIMESTAMPTZ,
    last_block_reason TEXT,
    crawl_delay_seconds INTEGER,
    budget_period_start TIMESTAMPTZ,
    budget_used INTEGER NOT NULL DEFAULT 0,
    budget_limit INTEGER NOT NULL DEFAULT 200,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE "SourceRun" ADD COLUMN IF NOT EXISTS "verdict" TEXT;
ALTER TABLE "SourceRun" ADD COLUMN IF NOT EXISTS "blockedCount" INTEGER NOT NULL DEFAULT 0;
ALTER TABLE "SourceRun" ADD COLUMN IF NOT EXISTS "blockReason" TEXT;

-- +goose Down
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "verdict";
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "blockedCount";
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "blockReason";
DROP TABLE host_retrieval_state;
