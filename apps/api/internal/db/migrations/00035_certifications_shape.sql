-- +goose Up
-- Adds certifications as a third configurable resume category alongside
-- skills and projects: an on/off switch plus a min/max entry count, mirroring
-- the projects columns added in 00034.
--
-- 0 means "unlimited / no limit" for "certificationsMin" and
-- "certificationsMax", same convention as "projectsMin"/"projectsMax". The
-- defaults reproduce current behaviour (certifications always render, no
-- count limit), so an upgraded install generates identical documents.
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "certificationsEnabled" boolean NOT NULL DEFAULT true;
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "certificationsMin" integer NOT NULL DEFAULT 0 CHECK ("certificationsMin" BETWEEN 0 AND 20);
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "certificationsMax" integer NOT NULL DEFAULT 0 CHECK ("certificationsMax" BETWEEN 0 AND 20);

-- +goose Down
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "certificationsEnabled";
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "certificationsMin";
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "certificationsMax";
