-- +goose Up
CREATE TABLE "Contact" (
    "id"              uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    "name"            text NOT NULL,
    "email"           text,
    "company"         text,
    "role"            text,
    "linkedinUrl"     text,
    "githubUsername"  text,
    "source"          text NOT NULL DEFAULT 'csv',
    "createdAt"       timestamp(3) DEFAULT now() NOT NULL
);

CREATE TABLE "ContactConnection" (
    "id"               uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    "fromContactId"    uuid NOT NULL REFERENCES "Contact"("id") ON DELETE CASCADE,
    "toContactId"      uuid NOT NULL REFERENCES "Contact"("id") ON DELETE CASCADE,
    "relationshipType" text NOT NULL DEFAULT 'professional',
    "strength"         real NOT NULL DEFAULT 0.5,
    "createdAt"        timestamp(3) DEFAULT now() NOT NULL,
    CONSTRAINT "ContactConnection_no_self" CHECK ("fromContactId" <> "toContactId")
);

CREATE INDEX "Contact_company_idx" ON "Contact" USING btree ("company");
CREATE INDEX "Contact_githubUsername_idx" ON "Contact" USING btree ("githubUsername");
CREATE INDEX "ContactConnection_from_idx" ON "ContactConnection" USING btree ("fromContactId");
CREATE INDEX "ContactConnection_to_idx" ON "ContactConnection" USING btree ("toContactId");
CREATE UNIQUE INDEX "ContactConnection_pair_idx" ON "ContactConnection" USING btree ("fromContactId", "toContactId");

-- +goose Down
DROP TABLE IF EXISTS "ContactConnection";
DROP TABLE IF EXISTS "Contact";
