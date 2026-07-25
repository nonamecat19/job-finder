package seed

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type sourceRunSeed struct {
	sourceKey string
	searchID  string
}

var sourceRuns = []sourceRunSeed{
	{sourceKey: "djinni", searchID: "seed:djinni:go-remote"},
	{sourceKey: "djinni", searchID: "seed:djinni:senior-react"},
	{sourceKey: "remotive", searchID: "seed:remotive:go-remote"},
	{sourceKey: "remotive", searchID: "seed:remotive:senior-react"},
	{sourceKey: "adzuna", searchID: "seed:adzuna:berlin"},
	{sourceKey: "adzuna", searchID: "seed:adzuna:fintech"},
	{sourceKey: "dou", searchID: "seed:dou:go-backend"},
	{sourceKey: "dou", searchID: "seed:dou:fullstack"},
	{sourceKey: "arbeitnow", searchID: "seed:arbeitnow:python"},
	{sourceKey: "arbeitnow", searchID: "seed:arbeitnow:k8s"},
	{sourceKey: "jooble", searchID: "seed:jooble:microservices"},
	{sourceKey: "jooble", searchID: "seed:jooble:frontend-lead"},
	{sourceKey: "glassdoor", searchID: "seed:glassdoor:ml-engineer"},
	{sourceKey: "indeed", searchID: "seed:indeed:sre"},
	{sourceKey: "indeed", searchID: "seed:indeed:backend-python"},
	{sourceKey: "wellfound", searchID: "seed:wellfound:golang-engineer"},
}

// seedSourceRuns links each run to the JobSource row lazily created by
// jobsources.Service.GetByKey in cmd/seed/main.go, looked up by key since
// that row's id isn't known ahead of time.
func seedSourceRuns(ctx context.Context, pool *pgxpool.Pool, q *sqlcgen.Queries) error {
	// Idempotency: check if seed source runs already exist.
	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM "SourceRun" WHERE "searchId" LIKE 'seed:%'`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		slog.Info("seed: source runs already exist, skipping")
		return nil
	}

	sourceIDs := make(map[string]sqlcgen.JobSource, len(sourceRuns))
	for _, sr := range sourceRuns {
		if _, ok := sourceIDs[sr.sourceKey]; ok {
			continue
		}
		source, err := q.GetJobSourceByKey(ctx, sr.sourceKey)
		if err != nil {
			return fmt.Errorf("seed: source runs: job source '%s' not found (run jobsources seeding first): %w", sr.sourceKey, err)
		}
		sourceIDs[sr.sourceKey] = source
	}

	for _, sr := range sourceRuns {
		searchID := sr.searchID
		_, err = q.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
			SourceId: sourceIDs[sr.sourceKey].ID,
			SearchId: &searchID,
		})
		if err != nil {
			return err
		}
	}

	slog.Info("seed: created source runs", "count", len(sourceRuns))
	return nil
}
