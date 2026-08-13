//go:build live

package application

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/scraping"
)

func TestLive_CompanyPage(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	gw, err := llm.NewProviders(cfg)
	if err != nil {
		t.Fatalf("llm new: %v", err)
	}

	scrapingSvc := scraping.New()
	defer scrapingSvc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const aboutURL = "https://www.ycombinator.com/about"

	html, err := scrapingSvc.FetchHTML(ctx, aboutURL, nil)
	if err != nil {
		t.Skipf("fetch %s failed (network/site unavailable): %v", aboutURL, err)
	}
	text := extractPageText(html)
	if text == "" {
		t.Fatal("expected non-empty flattened page text")
	}

	router := llm.NewRouter("recruiter", gw)
	contacts, err := ExtractCompanyPageContacts(ctx, router, "", text)
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	t.Logf("resolved %d contact(s) from %s", len(contacts), aboutURL)
	for _, c := range contacts {
		t.Logf("  %s (%s) source=%s confidence=%.2f", c.Name, ptrString(c.Title), c.Source, c.Confidence)
	}
}

func TestLive_LinkedIn(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.LinkedInScrapeEnabled {
		t.Skip("LINKEDIN_SCRAPE_ENABLED is not set to true — LinkedIn is opt-in (FR-004)")
	}

	gw, err := llm.NewProviders(cfg)
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

	router := llm.NewRouter("recruiter", gw)
	contacts, err := ExtractLinkedInContacts(ctx, router, "", text)
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
