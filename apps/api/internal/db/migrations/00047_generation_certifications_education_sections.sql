-- +goose Up
-- The ranked generation workspace gains certifications and education as
-- interactive sections (matching skills/projects), and every section gains
-- an `enabled` flag so the user can switch one off for this run without
-- losing its selection state.
ALTER TABLE generation_sections DROP CONSTRAINT generation_sections_kind_check;
ALTER TABLE generation_sections
    ADD CONSTRAINT generation_sections_kind_check
    CHECK (kind IN ('summary', 'experience', 'skills', 'projects', 'certifications', 'education'));

ALTER TABLE generation_sections ADD COLUMN enabled boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE generation_sections DROP COLUMN enabled;
DELETE FROM generation_sections WHERE kind IN ('certifications', 'education');
ALTER TABLE generation_sections DROP CONSTRAINT generation_sections_kind_check;
ALTER TABLE generation_sections
    ADD CONSTRAINT generation_sections_kind_check
    CHECK (kind IN ('summary', 'experience', 'skills', 'projects'));
