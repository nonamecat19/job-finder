-- +goose Up
-- The ranked generation workspace gained a projects section: projects are a
-- tailoring target like skills, and leaving them out meant the one section the
-- user could not review before export.
ALTER TABLE generation_sections DROP CONSTRAINT generation_sections_kind_check;
ALTER TABLE generation_sections
    ADD CONSTRAINT generation_sections_kind_check
    CHECK (kind IN ('summary', 'experience', 'skills', 'projects'));

-- +goose Down
DELETE FROM generation_sections WHERE kind = 'projects';
ALTER TABLE generation_sections DROP CONSTRAINT generation_sections_kind_check;
ALTER TABLE generation_sections
    ADD CONSTRAINT generation_sections_kind_check
    CHECK (kind IN ('summary', 'experience', 'skills'));
