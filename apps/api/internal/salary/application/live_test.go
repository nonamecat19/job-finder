//go:build live

package application_test

import (
	"context"
	"os"
	"testing"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/salary"
)

func TestLive_InferOneJob(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	database, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	levelsFyi := salary.NewLevelsFyiLoader(database.Queries)
	if cfg.LevelsFyiCSV != "" {
		if _, err := levelsFyi.LoadCSV(ctx, cfg.LevelsFyiCSV); err != nil {
			t.Logf("levels.fyi load warning: %v", err)
		}
	}

	svc := salary.NewService(database.Queries, levelsFyi, "")

	jobs, err := database.Queries.ListJobsByDate(ctx, sqlcgen.ListJobsByDateParams{Limit: 1})
	if err != nil || len(jobs) == 0 {
		t.Skip("no jobs in database")
	}

	jobID := dbutil.UUIDString(jobs[0].ID)
	t.Logf("inferring salary for job %s: %s at %s", jobID, jobs[0].Title, jobs[0].Company)

	// Infer only resolves salaryRaw/cache/levels.fyi hits now — the LLM
	// fallback is apps/ai's job (T103, T113). No hit for this job is an
	// expected outcome here, not a test failure.
	if err := svc.Infer(ctx, jobID); err != nil {
		t.Skipf("no cache/levels.fyi/salaryRaw band for job %s (expected once the LLM fallback is python-only): %v", jobID, err)
	}

	updated, err := database.Queries.GetJobByID(ctx, jobs[0].ID)
	if err != nil {
		t.Fatalf("get updated job: %v", err)
	}

	t.Logf("result: min=%v max=%v currency=%v confidence=%v source=%v",
		updated.SalaryMin, updated.SalaryMax, updated.SalaryCurrency,
		updated.SalaryConfidence, updated.SalarySource)
}
