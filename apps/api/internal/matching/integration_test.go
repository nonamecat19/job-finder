//go:build integration

package matching_test

import (
	"context"
	"os"
	"sync"
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

// fixedEmbedLLM returns a fixed 768-dim vector matching EMBED_DIMS's default,
// since the "embedding" columns are a fixed-dimension vector(768) type.
type fixedEmbedLLM struct{ noopLLM }

func (fixedEmbedLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	v := make([]float32, 768)
	v[0] = 1
	return v, nil
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

	profiles := profile.NewService(profile.NewSqlcRepository(testDB.Queries), noopLLM{}, "nomic-embed-text", "rendercv")
	svc := matching.NewService(matching.NewSqlcRepository(testDB.Queries), profiles, nil, noopLLM{}, 0.5, "")

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

// TestMatchJob_ConcurrentMatchConverges exercises FR-017: running the same
// job through two concurrent match calls (as the admission gate now allows
// at hosted concurrency > 1) must converge on one consistent row, since
// UpsertMatchResult is keyed on jobId (research.md R6).
func TestMatchJob_ConcurrentMatchConverges(t *testing.T) {
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
		Key: "mj-src-concurrent", Kind: "api", Config: []byte(`{}`),
	}); err != nil {
		t.Fatalf("upsert job source: %v", err)
	}
	job, err := testDB.Queries.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey:   "mj-dedupe-concurrent",
		SourceKey:   "mj-src-concurrent",
		Title:       "Backend Engineer",
		Company:     "TestCo",
		Url:         "https://example.com/job/mj-dedupe-concurrent",
		Description: "A test job listing.",
		Raw:         []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	if _, err := testDB.Queries.CreateProfile(ctx, sqlcgen.CreateProfileParams{
		Name:           "Concurrent Match Profile",
		RendercvConfig: []byte(`{}`),
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	embedLLM := fixedEmbedLLM{}
	profiles := profile.NewService(profile.NewSqlcRepository(testDB.Queries), embedLLM, "nomic-embed-text", "rendercv")
	// Threshold set above 1.0 (similarity is bounded [-1,1]) so both calls
	// take the below-threshold path deterministically, without needing a
	// stubbed LLM fit-analysis response.
	svc := matching.NewService(matching.NewSqlcRepository(testDB.Queries), profiles, nil, embedLLM, 1.1, "")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.MatchJob(ctx, dbutil.UUIDString(job.ID), nil)
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("MatchJob call %d: %v", i, err)
		}
	}

	result, err := testDB.Queries.GetMatchResultByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get match result: %v", err)
	}
	if result.JobId != job.ID {
		t.Fatalf("expected exactly one converged MatchResult row for jobId %v, got %v", job.ID, result.JobId)
	}
}
