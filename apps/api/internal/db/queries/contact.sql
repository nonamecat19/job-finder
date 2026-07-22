-- name: InsertContact :one
INSERT INTO "Contact" ("name", "email", "company", "role", "linkedinUrl", "githubUsername", "source")
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetContactByID :one
SELECT * FROM "Contact" WHERE "id" = $1;

-- name: ListContacts :many
SELECT * FROM "Contact" ORDER BY "createdAt" DESC;

-- name: ListContactsByCompany :many
SELECT * FROM "Contact" WHERE "company" = $1 ORDER BY "name";

-- name: FindContactsByGithubUsername :many
SELECT * FROM "Contact" WHERE "githubUsername" = $1;

-- name: DeleteAllContacts :execrows
DELETE FROM "Contact";

-- name: InsertContactConnection :one
INSERT INTO "ContactConnection" ("fromContactId", "toContactId", "relationshipType", "strength")
VALUES ($1, $2, $3, $4)
ON CONFLICT ("fromContactId", "toContactId") DO UPDATE
SET "relationshipType" = EXCLUDED."relationshipType",
    "strength" = EXCLUDED."strength"
RETURNING *;

-- name: ListConnections :many
SELECT * FROM "ContactConnection" ORDER BY "createdAt" DESC;

-- name: GetConnectionsFrom :many
SELECT * FROM "ContactConnection" WHERE "fromContactId" = $1;

-- name: GetConnectionsTo :many
SELECT * FROM "ContactConnection" WHERE "toContactId" = $1;

-- name: DeleteAllConnections :execrows
DELETE FROM "ContactConnection";

-- name: FindContactsAtCompany :many
SELECT * FROM "Contact" WHERE "company" ILIKE '%' || $1 || '%' ORDER BY "name";
