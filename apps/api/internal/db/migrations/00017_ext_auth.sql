-- +goose Up
-- Browser-extension auth (014-autofill-extension): bootstrap codes are
-- issued by the dashboard and exchanged once for a token pair; refresh
-- tokens are opaque and rotated on every use. Only hashes are stored —
-- the plaintext code/token never touches the database, so a DB leak alone
-- cannot be replayed against the exchange/refresh endpoints.
CREATE TABLE "ExtBootstrapCode" (
  "id"        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "codeHash"  text NOT NULL UNIQUE,
  "createdAt" timestamp(3) NOT NULL DEFAULT now(),
  "expiresAt" timestamp(3) NOT NULL,
  "usedAt"    timestamp(3)
);

CREATE INDEX "ExtBootstrapCode_expiresAt_idx" ON "ExtBootstrapCode"("expiresAt");

CREATE TABLE "ExtRefreshToken" (
  "id"          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "tokenHash"   text NOT NULL UNIQUE,
  "createdAt"   timestamp(3) NOT NULL DEFAULT now(),
  "expiresAt"   timestamp(3) NOT NULL,
  "revokedAt"   timestamp(3),
  "rotatedToId" uuid REFERENCES "ExtRefreshToken"("id")
);

CREATE INDEX "ExtRefreshToken_expiresAt_idx" ON "ExtRefreshToken"("expiresAt");

-- +goose Down
DROP TABLE IF EXISTS "ExtRefreshToken";
DROP TABLE IF EXISTS "ExtBootstrapCode";
