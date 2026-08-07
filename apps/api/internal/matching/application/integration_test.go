//go:build integration

package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/platform/llm"
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

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
