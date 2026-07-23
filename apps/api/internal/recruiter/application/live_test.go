//go:build live

package application

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/scraping"
)

// TestLive_CompanyPage hits a real company website's About/Team page and
// runs the actual local LLM over it. This is the only test that catches
// real-world markup drift in the company-page source; it needs a running
// Ollama and network access, so it's excluded from the default `go test`
// run (build tag "live") and skips cleanly when the prerequisites aren't
// present.
func TestLive_CompanyPage(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	llmProvider, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("llm new: %v", err)
	}

	scrapingSvc := scraping.New()
	defer scrapingSvc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A stable, real company About page — swap for a target of your choice
	// when investigating a specific site's markup.
	const aboutURL = "https://www.ycombinator.com/about"

	html, err := scrapingSvc.FetchHTML(ctx, aboutURL, nil)
	if err != nil {
		t.Skipf("fetch %s failed (network/site unavailable): %v", aboutURL, err)
	}
	text := extractPageText(html)
	if text == "" {
		t.Fatal("expected non-empty flattened page text")
	}

	contacts, err := ExtractCompanyPageContacts(ctx, llmProvider, cfg.ModelOr(""), text)
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	t.Logf("resolved %d contact(s) from %s", len(contacts), aboutURL)
	for _, c := range contacts {
		t.Logf("  %s (%s) source=%s confidence=%.2f", c.Name, ptrString(c.Title), c.Source, c.Confidence)
	}
}

// TestLive_LinkedIn hits the real public LinkedIn People page for a
// company. Skips unless the operator has explicitly opted in via
// LINKEDIN_SCRAPE_ENABLED=true (FR-004) — running this test at all is a
// LinkedIn request, so it must never fire from a default `go test -tags
// live` run.
func TestLive_LinkedIn(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.LinkedInScrapeEnabled {
		t.Skip("LINKEDIN_SCRAPE_ENABLED is not set to true — LinkedIn is opt-in (FR-004)")
	}

	llmProvider, err := llm.New(cfg)
	if err != nil {
		t.Fatalf("llm new: %v", err)
	}

	scrapingSvc := scraping.New()
	defer scrapingSvc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const peopleURL = "https://www.linkedin.com/company/y-combinator/people/"

	html, err := scrapingSvc.FetchHTML(ctx, peopleURL, nil)
	if err != nil {
		t.Skipf("fetch %s failed (blocked/unreachable): %v", peopleURL, err)
	}
	text := extractPageText(html)
	if text == "" {
		t.Skip("empty page text — likely an auth wall or markup change")
	}

	contacts, err := ExtractLinkedInContacts(ctx, llmProvider, cfg.ModelOr(""), text)
	if err != nil {
		t.Fatalf("ExtractLinkedInContacts: %v", err)
	}
	t.Logf("resolved %d contact(s) from %s", len(contacts), peopleURL)
	for _, c := range contacts {
		t.Logf("  %s (%s) source=%s confidence=%.2f", c.Name, ptrString(c.Title), c.Source, c.Confidence)
	}
}

func ptrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
