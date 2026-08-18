-- +goose Up
-- IF NOT EXISTS because this migration's Down deliberately keeps the audit
-- table (see below): after a rollback the table is still there, so a plain
-- CREATE TABLE would make rolling forward again fail with "relation already
-- exists" — the exact situation a rollback exists to recover from.
CREATE TABLE IF NOT EXISTS "DjinniLegacySubAudit" (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    "subscriptionId" UUID NOT NULL,
    name TEXT,
    url TEXT NOT NULL,
    "deletedAt" TIMESTAMPTZ NOT NULL DEFAULT now()
);

WITH deleted AS (
    DELETE FROM "Subscription"
    WHERE "sourceKey" = 'djinni'
      AND "url" LIKE '%/my/dashboard/subs/%'
    RETURNING "id", "name", "url", "createdAt"
)
INSERT INTO "DjinniLegacySubAudit" ("subscriptionId", "name", "url", "deletedAt")
SELECT "id", "name", "url", now() FROM deleted;

-- +goose Down
-- Deletion is irreversible — audit rows remain as the record, so this drops
-- nothing at all. The Up above is written to tolerate the table it leaves.
