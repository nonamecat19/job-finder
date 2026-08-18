-- name: GetResumeShapeSetting :one
SELECT * FROM "ResumeShapeSetting" WHERE "id" = 'default';

-- name: UpdateResumeShapeSetting :one
UPDATE "ResumeShapeSetting"
SET "summaryLines" = $1,
    "skillsEnabled" = $2,
    "skillsMaxGroups" = $3,
    "experienceBulletsMin" = $4,
    "experienceBulletsMax" = $5,
    "targetPages" = $6,
    "projectsEnabled" = $7,
    "projectsMin" = $8,
    "projectsMax" = $9,
    "projectBulletsMax" = $10,
    "certificationsEnabled" = $11,
    "certificationsMin" = $12,
    "certificationsMax" = $13,
    "fontSize" = $14,
    "experienceEnabled" = $15,
    "summaryEnabled" = $16,
    "educationEnabled" = $17,
    "updatedAt" = now()
WHERE "id" = 'default'
RETURNING *;
