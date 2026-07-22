-- name: UpsertJobContact :one
-- Insert-or-update on (jobId, source, name) per FR-013: a re-run of
-- resolution for the same job/source/name updates the existing row (fresh
-- title/channels/confidence/fetchedAt) instead of duplicating it.
INSERT INTO "JobContact" ("jobId", name, title, "linkedInUrl", email, phone, source, confidence, "fetchedAt")
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT ("jobId", source, name) DO UPDATE SET
  title = EXCLUDED.title,
  "linkedInUrl" = EXCLUDED."linkedInUrl",
  email = EXCLUDED.email,
  phone = EXCLUDED.phone,
  confidence = EXCLUDED.confidence,
  "fetchedAt" = now()
RETURNING *;

-- name: ListJobContactsByJob :many
-- Deterministic best-first order for FR-010/SC-010: confidence descending,
-- then a fixed source priority (posting beats company-page beats linkedin,
-- reflecting how directly each source names the person as owning the req),
-- then name as the final tie-break so identical inputs always render the
-- same order.
SELECT * FROM "JobContact"
WHERE "jobId" = $1
ORDER BY
  confidence DESC,
  CASE source
    WHEN 'posting' THEN 0
    WHEN 'company-page' THEN 1
    WHEN 'linkedin' THEN 2
    ELSE 3
  END,
  name ASC;
