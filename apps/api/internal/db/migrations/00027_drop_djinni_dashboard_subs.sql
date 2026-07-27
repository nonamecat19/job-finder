-- +goose Up
CREATE TABLE "DjinniLegacySubAudit" (
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
-- Deletion is irreversible — audit rows remain as the record.
