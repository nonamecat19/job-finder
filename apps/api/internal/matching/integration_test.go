//go:build integration

package matching_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/profile"
)

type noopLLM struct{}

func (noopLLM) ModelName() string { return "noop" }
func (noopLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (noopLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (noopLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func TestMatchJob_SkipsProfileWithoutConfig(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	testDB, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer testDB.Close()

	// Other integration suites truncate these same tables in parallel; take turns.
	unlock, err := dbtest.LockSharedDB(ctx, testDB.Pool)
	if err != nil {
		t.Fatalf("lock shared db: %v", err)
	}
	defer unlock()

	for _, tbl := range []string{"MatchResult", "Job", "JobSource", "Profile"} {
		if _, err := testDB.Pool.Exec(ctx, `TRUNCATE TABLE "`+tbl+`" CASCADE`); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}

	if err := testDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key: "mj-src", Kind: "api", Config: []byte(`{}`),
	}); err != nil {
		t.Fatalf("upsert job source: %v", err)
	}
	job, err := testDB.Queries.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey:   "mj-dedupe-1",
		SourceKey:   "mj-src",
		Title:       "Backend Engineer",
		Company:     "TestCo",
		Url:         "https://example.com/job/mj-dedupe-1",
		Description: "A test job listing.",
		Raw:         []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	// Profile with no rendercv config at all — the hard gate should skip it.
	if _, err := testDB.Queries.CreateProfile(ctx, sqlcgen.CreateProfileParams{
		Name: "No Config Profile",
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	profiles := profile.NewService(testDB.Queries, noopLLM{}, "nomic-embed-text", "rendercv")
	svc := matching.NewService(testDB.Queries, profiles, noopLLM{}, 0.5, "")

	_, err = svc.MatchJob(ctx, dbutil.UUIDString(job.ID), nil)
	if err != matching.ErrNoProfileConfig {
		t.Fatalf("expected ErrNoProfileConfig, got %v", err)
	}

	result, err := testDB.Queries.GetMatchResultByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if result.Summary == nil || *result.Summary != "no profile config" {
		t.Fatalf("expected sentinel summary, got %v", result.Summary)
	}
}
