-- +goose Up
-- Adds a configurable body font size to the resume shape settings, mirroring
-- the pattern used for certifications in 00035. The default of 10pt matches
-- the classic rendercv theme's own default, so an upgraded install generates
-- identical documents until the user changes it.
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "fontSize" integer NOT NULL DEFAULT 10 CHECK ("fontSize" BETWEEN 8 AND 14);

-- +goose Down
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "fontSize";
