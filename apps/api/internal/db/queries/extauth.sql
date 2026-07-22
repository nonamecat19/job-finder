-- name: InsertBootstrapCode :one
INSERT INTO "ExtBootstrapCode" ("codeHash", "expiresAt")
VALUES ($1, $2)
RETURNING *;

-- name: ConsumeBootstrapCode :one
UPDATE "ExtBootstrapCode"
SET "usedAt" = now()
WHERE "codeHash" = $1 AND "usedAt" IS NULL AND "expiresAt" > now()
RETURNING *;

-- name: InsertRefreshToken :one
INSERT INTO "ExtRefreshToken" ("tokenHash", "expiresAt")
VALUES ($1, $2)
RETURNING *;

-- name: GetActiveRefreshToken :one
SELECT * FROM "ExtRefreshToken"
WHERE "tokenHash" = $1 AND "revokedAt" IS NULL AND "expiresAt" > now();

-- name: RevokeRefreshToken :exec
UPDATE "ExtRefreshToken"
SET "revokedAt" = now(), "rotatedToId" = $2
WHERE "id" = $1;

-- name: DeleteExpiredBootstrapCodes :execrows
DELETE FROM "ExtBootstrapCode" WHERE "expiresAt" < now() - interval '1 day';
