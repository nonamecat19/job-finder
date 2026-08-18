-- +goose Up
-- Extends the resume-shape on/off switches (skills/projects/certifications)
-- to the remaining sections: experience, summary, and the new education
-- section. Booleans only — experience/summary already have their own shape
-- knobs (experienceBulletsMin/Max, summaryLines) and education has no
-- ranking model to cap against, so no min/max columns are needed here.
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "experienceEnabled" boolean NOT NULL DEFAULT true;
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "summaryEnabled" boolean NOT NULL DEFAULT true;
ALTER TABLE "ResumeShapeSetting" ADD COLUMN "educationEnabled" boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "experienceEnabled";
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "summaryEnabled";
ALTER TABLE "ResumeShapeSetting" DROP COLUMN "educationEnabled";
