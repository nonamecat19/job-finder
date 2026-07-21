-- +goose Up
CREATE TABLE "FreshMatchNotification" (
    "id"             uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    "jobId"          uuid NOT NULL REFERENCES "Job"("id") ON DELETE CASCADE,
    "matchResultId"  uuid NOT NULL REFERENCES "MatchResult"("id") ON DELETE CASCADE,
    "profileId"      uuid NOT NULL REFERENCES "Profile"("id") ON DELETE CASCADE,
    "fresh"          boolean NOT NULL,
    "seen"           boolean DEFAULT false NOT NULL,
    "createdAt"      timestamp (3) DEFAULT now() NOT NULL
);

CREATE INDEX "FreshMatchNotification_profileId_createdAt_idx"
    ON "FreshMatchNotification" USING btree ("profileId", "createdAt" DESC);

CREATE INDEX "FreshMatchNotification_jobId_idx"
    ON "FreshMatchNotification" USING btree ("jobId");

-- +goose Down
DROP TABLE IF EXISTS "FreshMatchNotification";
