-- +goose Up
-- Backfill "subscriptionId" for jobs left unattributed by the pre-fix
-- RecordJobRepost (which only bumped seenCount, never wrote subscriptionId
-- on a repost hit). Best-effort: a job can't be traced to the specific
-- subscription run that (re)found it, so where more than one subscription
-- shares the job's sourceKey we deterministically pick the oldest one —
-- enough to make "filter by Subscription" show something rather than
-- nothing, though it may not be the original match.
WITH candidate AS (
    SELECT DISTINCT ON (s."sourceKey") s."sourceKey", s.id AS "subscriptionId"
    FROM "Subscription" s
    ORDER BY s."sourceKey", s."createdAt" ASC
)
UPDATE "Job" j
SET "subscriptionId" = c."subscriptionId"
FROM candidate c
WHERE j."subscriptionId" IS NULL
  AND j."sourceKey" = c."sourceKey";

-- +goose Down
-- Backfill is best-effort and not reversible to the prior (also incomplete) state.
