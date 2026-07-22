-- name: GetCompanySignals :many
SELECT * FROM "CompanySignal" WHERE "companyId" = $1 ORDER BY kind;

-- name: UpsertCompanySignal :one
INSERT INTO "CompanySignal" ("companyId", kind, value, source, "fetchedAt", raw)
VALUES ($1, $2, $3, $4, now(), $5)
ON CONFLICT ("companyId", kind) DO UPDATE SET
  value = EXCLUDED.value,
  source = EXCLUDED.source,
  "fetchedAt" = now(),
  raw = EXCLUDED.raw
RETURNING *;

-- name: GetCompanySignalByKind :one
SELECT * FROM "CompanySignal" WHERE "companyId" = $1 AND kind = $2;

-- name: DeleteCompanySignals :execrows
DELETE FROM "CompanySignal" WHERE "companyId" = $1;
