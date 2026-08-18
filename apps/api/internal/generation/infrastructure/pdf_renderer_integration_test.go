//go:build integration

package infrastructure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/testinfra"
)

func renderer(t *testing.T) *HtmlPdfRenderer {
	t.Helper()

	if chromeErr != nil {
		t.Fatalf("start chrome: %v", chromeErr)
	}
	scraper := scraping.NewWithRemoteBrowser(chromeWS)
	t.Cleanup(scraper.Close)

	r, err := NewHtmlPdfRenderer(scraper, t.TempDir())
	if err != nil {
		t.Fatalf("NewHtmlPdfRenderer: %v", err)
	}
	return r
}

func str(s string) *string { return &s }

func sampleResume() dto.JsonResume {
	return dto.JsonResume{
		Basics: &dto.ResumeBasics{
			Name:    str("Ada Lovelace"),
			Label:   str("Senior Go Engineer"),
			Email:   str("ada@example.com"),
			Summary: str("Backend engineer with a long history of building analytical engines."),
		},
		Work: []dto.ResumeWork{{
			Name:       "Analytical Engines Ltd",
			Position:   str("Principal Engineer"),
			StartDate:  str("2023-01"),
			EndDate:    str("2026-01"),
			Highlights: []string{"Cut ingest latency by half", "Wrote the first program"},
		}},
		Skills: []dto.ResumeSkill{{Name: "Go", Keywords: []string{"pgx", "chromedp"}}},
	}
}

func readPDF(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered pdf at %s: %v", path, err)
	}
	return body
}

func TestRenderResumeProducesAPDF(t *testing.T) {
	r := renderer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	path, err := r.RenderResume(ctx, sampleResume(), "resume.pdf")
	if err != nil {
		t.Fatalf("RenderResume: %v", err)
	}

	body := readPDF(t, path)
	if !strings.HasPrefix(string(body), "%PDF-") {
		t.Fatalf("rendered file does not start with a PDF header: %.16q", body)
	}

	if len(body) < 3000 {
		t.Fatalf("rendered pdf is %d bytes, too small to contain the résumé", len(body))
	}
}

func TestRenderCoverLetterProducesAPDF(t *testing.T) {
	r := renderer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const letter = "Dear hiring team,\n\nI am writing about the Senior Go Engineer role.\n\nSincerely,\nAda"
	path, err := r.RenderCoverLetter(ctx, letter, str("Ada Lovelace"), "Acme", "Senior Go Engineer", "letter.pdf")
	if err != nil {
		t.Fatalf("RenderCoverLetter: %v", err)
	}

	body := readPDF(t, path)
	if !strings.HasPrefix(string(body), "%PDF-") {
		t.Fatalf("rendered file does not start with a PDF header: %.16q", body)
	}
	if len(body) < 1500 {
		t.Fatalf("rendered pdf is %d bytes, too small to contain the letter", len(body))
	}
}

func TestRenderResumeIsDeterministicInSize(t *testing.T) {
	r := renderer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	first, err := r.RenderResume(ctx, sampleResume(), "first.pdf")
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := r.RenderResume(ctx, sampleResume(), "second.pdf")
	if err != nil {
		t.Fatalf("second render: %v", err)
	}

	if a, b := len(readPDF(t, first)), len(readPDF(t, second)); a != b {
		t.Fatalf("rendering the same résumé twice produced %d and %d bytes", a, b)
	}
}

func TestRenderResumeCreatesTheOutputDirectory(t *testing.T) {
	if chromeErr != nil {
		t.Fatalf("start chrome: %v", chromeErr)
	}
	scraper := scraping.NewWithRemoteBrowser(chromeWS)
	t.Cleanup(scraper.Close)

	outDir := t.TempDir() + "/nested/documents"
	r, err := NewHtmlPdfRenderer(scraper, outDir)
	if err != nil {
		t.Fatalf("NewHtmlPdfRenderer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	path, err := r.RenderResume(ctx, sampleResume(), "resume.pdf")
	if err != nil {
		t.Fatalf("RenderResume into a directory that does not exist: %v", err)
	}
	if !strings.HasPrefix(path, outDir) {
		t.Fatalf("rendered to %s, want a file under %s", path, outDir)
	}
}

var (
	chromeWS  string
	chromeErr error
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	chromeWS, chromeErr = testinfra.ChromeWebSocketURL(ctx, 0)
	cancel()

	code := m.Run()
	if chromeErr != nil {
		fmt.Fprintf(os.Stderr, "chrome container: %v\n", chromeErr)
	}
	os.Exit(code)
}
