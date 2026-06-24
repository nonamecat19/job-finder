package seed

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

func seedSavedSearches(ctx context.Context, q *sqlcgen.Queries) error {
	existing, err := q.ListSavedSearches(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		slog.Info("seed: saved searches already exist, skipping")
		return nil
	}

	type searchDef struct {
		name  string
		query map[string]any
		cron  string
		on    bool
	}
	searches := []searchDef{
		{
			name: "Go Remote Jobs",
			query: map[string]any{
				"keywords": "golang",
				"remote":   true,
				"sources":  []string{"djinni", "remotive"},
			},
			cron: "0 */6 * * *",
			on:   true,
		},
		{
			name: "Senior React",
			query: map[string]any{
				"keywords": "react senior",
			},
			cron: "0 */12 * * *",
			on:   true,
		},
		{
			name: "Berlin Tech",
			query: map[string]any{
				"keywords": "software engineer",
				"location": "Berlin",
			},
			cron: "0 9 * * 1",
			on:   true,
		},
		{
			name: "Ukrainian Startups",
			query: map[string]any{
				"keywords": "developer",
				"country":  "ua",
				"remote":   true,
			},
			cron: "0 8 * * *",
			on:   false,
		},
	}

	for _, s := range searches {
		queryJSON, err := json.Marshal(s.query)
		if err != nil {
			return err
		}
		_, err = q.CreateSavedSearch(ctx, sqlcgen.CreateSavedSearchParams{
			Name:    s.name,
			Query:   queryJSON,
			Cron:    s.cron,
			Enabled: s.on,
		})
		if err != nil {
			return err
		}
	}

	slog.Info("seed: created saved searches", "count", len(searches))
	return nil
}
