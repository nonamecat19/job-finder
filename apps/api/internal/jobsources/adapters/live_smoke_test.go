//go:build live

package adapters

// Run with: go test -tags live ./internal/jobsources/adapters/... -run Live -v
// Not run by default (requires internet + upstream APIs to be healthy) — this
// is a manual parity smoke test, not part of `go test ./...`.

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/scraping"
)

func TestLive_Remotive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	jobs, err := RemotiveAdapter{}.Search(ctx, dto.SearchQuery{Keywords: "golang"}, nil)
	if err != nil {
		t.Fatalf("remotive search failed: %v", err)
	}
	t.Logf("remotive returned %d jobs", len(jobs))
	if len(jobs) > 0 {
		t.Logf("sample: %+v", jobs[0])
	}
}

func TestLive_Arbeitnow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	jobs, err := ArbeitnowAdapter{}.Search(ctx, dto.SearchQuery{Keywords: "engineer"}, nil)
	if err != nil {
		t.Fatalf("arbeitnow search failed: %v", err)
	}
	t.Logf("arbeitnow returned %d jobs", len(jobs))
}

func TestLive_WorkUa(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := WorkUaAdapter{Scraping: scraping.New(nil)}
	jobs, err := a.Search(ctx, dto.SearchQuery{Keywords: "php"}, nil)
	if err != nil {
		t.Fatalf("workua search failed: %v", err)
	}
	t.Logf("workua returned %d jobs", len(jobs))
	if len(jobs) > 0 {
		t.Logf("sample: %+v", jobs[0])
	}
	if len(jobs) == 0 {
		t.Error("expected at least 1 job from live search")
	}
}

func TestLive_Adzuna_NoCreds(t *testing.T) {
	ctx := context.Background()
	_, err := AdzunaAdapter{}.Search(ctx, dto.SearchQuery{Keywords: "engineer"}, nil)
	if err == nil {
		t.Fatal("expected error without credentials")
	}
}
