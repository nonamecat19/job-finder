package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

func seedJobs(ctx context.Context, pool *pgxpool.Pool, _ *sqlcgen.Queries) error {
	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM "Job" WHERE "dedupeKey" LIKE 'seed:%'`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		slog.Info("seed: jobs already exist, skipping")
		return nil
	}

	const insertSQL = `INSERT INTO "Job" (
		"id", "dedupeKey", "sourceKey", "externalId", "title", "company", "location",
		"remote", "salaryRaw", "url", "description", "raw", "postedAt", "status"
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	for _, j := range jobs {
		raw := map[string]any{
			"sourceKey":   j.SourceKey,
			"title":       j.Title,
			"company":     j.Company,
			"location":    j.Location,
			"remote":      j.Remote,
			"salaryRaw":   j.SalaryRaw,
			"url":         j.URL,
			"description": j.Desc,
		}
		rawJSON, err := json.Marshal(raw)
		if err != nil {
			return err
		}

		id, err := dbutil.ParseUUID(j.UUID)
		if err != nil {
			return fmt.Errorf("invalid job uuid %q: %w", j.UUID, err)
		}

		dedupeKey := "seed:" + j.SourceKey + ":" + j.Title
		location := j.Location
		salaryRaw := j.SalaryRaw
		postedAt := pgtype.Timestamp{Time: j.PostedAt, Valid: !j.PostedAt.IsZero()}

		_, err = pool.Exec(ctx, insertSQL,
			id, dedupeKey, j.SourceKey, nil, j.Title, j.Company, &location,
			j.Remote, &salaryRaw, j.URL, j.Desc, rawJSON, postedAt, j.Status,
		)
		if err != nil {
			return fmt.Errorf("insert job %q: %w", j.Title, err)
		}
	}

	slog.Info("seed: created jobs", "count", len(jobs))
	return nil
}
