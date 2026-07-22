-- +goose Up
-- Ghost-job detector (005) repost signal: how many times has this exact
-- posting identity (dedupeKey) been re-seen across ingestion runs?
--
-- persistIfNew (ingestion/handler.go) already looks up a job by dedupeKey
-- before inserting and, on a hit, treats it as a no-op today — nothing
-- records that the posting was seen again. Without a counter, "repost count"
-- can only ever be 1 (Job.dedupeKey is UNIQUE), which is exactly the trap
-- documented in specs/005-ghost-job-detector/tasks.md's T008 note: a
-- `count(*) FROM "Job" GROUP BY "dedupeKey"` compiles and always returns 1.
--
-- "seenCount" makes every re-encounter of the same dedupeKey durable so the
-- ghost detector's repost signal (FR-002a/FR-003) is measurable at all.
ALTER TABLE "Job" ADD COLUMN "seenCount" integer NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE "Job" DROP COLUMN IF EXISTS "seenCount";
