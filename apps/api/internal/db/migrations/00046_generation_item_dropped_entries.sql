-- +goose Up
-- Per-skill inclusion inside a skill group. The workspace's skills section
-- was toggleable only a whole group at a time ("Backend: TypeORM, Prisma,
-- NestJS, …" in or out), so trimming one irrelevant skill meant dropping
-- thirty relevant ones with it. dropped_entries names the individual entries
-- of that group's `details` the user switched off; everything not named stays
-- in, so an empty array is the untouched group and the column needs no
-- backfill.
--
-- Entries are stored as their trimmed display text, matching how the domain
-- splits `details` on commas, rather than as indices: a rerun re-reads the
-- group from the master snapshot, and text survives a reordering of that
-- group where an index would silently drop the wrong skill.
--
-- Only meaningful for a profile-origin item in a skills section. Nothing
-- enforces that here (an item's kind lives on its section, not on the row),
-- and the write path is the one place that rejects it — same shape as the
-- existing edited_text rule.
ALTER TABLE generation_items
    ADD COLUMN dropped_entries text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE generation_items DROP COLUMN dropped_entries;
