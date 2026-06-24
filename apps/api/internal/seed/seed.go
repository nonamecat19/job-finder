// Package seed provides idempotent database seeding for local development.
// Run via: go run ./cmd/seed
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Options controls what the seeder does.
type Options struct {
	// Tables restricts seeding to the named tables. Empty means all.
	Tables []string
}

// job sources are not seed fixtures: they come from the adapter registry via
// jobsources.Service.Seed, which the caller runs before Run (and after Clean,
// if requested).
var allTables = []string{
	"profile", "job", "savedsearch",
	"application", "document", "matchresult", "sourcerun", "subscription",
}

// Run executes all seeders in FK-safe order.
func Run(ctx context.Context, pool *pgxpool.Pool, q *sqlcgen.Queries, opts Options) error {
	scope := opts.Tables
	if len(scope) == 0 {
		scope = allTables
	}
	need := func(name string) bool {
		for _, t := range scope {
			if t == name {
				return true
			}
		}
		return false
	}

	type step struct {
		name string
		fn   func(ctx context.Context, pool *pgxpool.Pool, q *sqlcgen.Queries) error
	}
	steps := []step{
		{"profile", func(ctx context.Context, _ *pgxpool.Pool, q *sqlcgen.Queries) error {
			return seedProfile(ctx, q)
		}},
		{"job", seedJobs},
		{"savedsearch", func(ctx context.Context, _ *pgxpool.Pool, q *sqlcgen.Queries) error {
			return seedSavedSearches(ctx, q)
		}},
		{"application", seedApplications},
		{"document", seedDocuments},
		{"matchresult", seedMatchResults},
		{"sourcerun", seedSourceRuns},
		{"subscription", func(ctx context.Context, _ *pgxpool.Pool, q *sqlcgen.Queries) error {
			return seedSubscriptions(ctx, q)
		}},
	}

	for _, s := range steps {
		if !need(s.name) {
			continue
		}
		if err := s.fn(ctx, pool, q); err != nil {
			return fmt.Errorf("seed: %s: %w", s.name, err)
		}
	}

	slog.Info("seed: complete", "tables", strings.Join(scope, ", "))
	return nil
}

// Clean truncates all tables in FK-safe order (children first). Callers that
// also seed job sources from the registry must call this before that, since
// it truncates JobSource too.
func Clean(ctx context.Context, pool *pgxpool.Pool) error {
	tables := []string{
		`"GeneratedDocument"`,
		`"MatchResult"`,
		`"Application"`,
		`"SourceRun"`,
		`"Subscription"`,
		`"Job"`,
		`"SavedSearch"`,
		`"JobSource"`,
		`"Profile"`,
	}
	for _, t := range tables {
		if _, err := pool.Exec(ctx, "TRUNCATE TABLE "+t+" CASCADE"); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}
	return nil
}
