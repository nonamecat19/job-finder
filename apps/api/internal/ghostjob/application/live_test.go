//go:build live

package application_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/platform/llm"
)

func TestLive_GhostScore(t *testing.T) {
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

	llmProvider, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("llm new: %v", err)
	}

	svc := ghostjob.NewService(database.Queries, llmProvider, cfg.ModelOr(cfg.LLMModelGhost))

	jobs, err := database.Queries.ListJobsByDate(ctx, sqlcgen.ListJobsByDateParams{Limit: 1})
	if err != nil || len(jobs) == 0 {
		t.Skip("no jobs in database")
	}

	jobID := dbutil.UUIDString(jobs[0].ID)
	t.Logf("scoring job %s: %s at %s", jobID, jobs[0].Title, jobs[0].Company)

	out, err := svc.ScoreJob(ctx, jobID)
	if err != nil {
		if err == ghostjob.ErrDeclinedToScore {
			t.Skip("this job's signals are all unknown; system correctly declined to score")
		}
		t.Fatalf("score job: %v", err)
	}

	if out.Score < 0 || out.Score > 100 {
		t.Errorf("expected score in 0-100, got %d", out.Score)
	}
	if out.Signals.Confidence < 0 || out.Signals.Confidence > 1 {
		t.Errorf("expected confidence in 0-1, got %v", out.Signals.Confidence)
	}
	if out.Model == "" {
		t.Error("expected a model identifier")
	}
	if strings.TrimSpace(out.Signals.Explanation) == "" {
		t.Error("expected a non-empty explanation")
	}
	t.Logf("score=%d confidence=%.2f explanation=%q", out.Score, out.Signals.Confidence, out.Signals.Explanation)
}
