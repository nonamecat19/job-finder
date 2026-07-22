-- name: GetCompanyByNormalizedName :one
SELECT * FROM "Company" WHERE "normalizedName" = $1;

-- name: UpsertCompany :one
INSERT INTO "Company" (name, "normalizedName", website, "firstSeenAt", "lastRefreshedAt")
VALUES ($1, $2, $3, now(), $4)
ON CONFLICT ("normalizedName") DO UPDATE SET
  name = EXCLUDED.name,
  website = COALESCE(EXCLUDED.website, "Company".website),
  "lastRefreshedAt" = COALESCE(EXCLUDED."lastRefreshedAt", "Company"."lastRefreshedAt")
RETURNING *;

-- name: UpdateCompanyLastRefreshed :exec
UPDATE "Company" SET "lastRefreshedAt" = now() WHERE id = $1;
