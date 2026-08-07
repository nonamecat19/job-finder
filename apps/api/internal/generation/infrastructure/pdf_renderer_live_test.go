//go:build live

package infrastructure

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/scraping"
)

func strp2(s string) *string { return &s }

func TestLive_HtmlPdfRenderer_RendersResume(t *testing.T) {
	outDir := t.TempDir()
	scrapingSvc := scraping.New()
	defer scrapingSvc.Close()

	renderer, err := NewHtmlPdfRenderer(scrapingSvc, outDir)
	if err != nil {
		t.Fatalf("NewHtmlPdfRenderer: %v", err)
	}

	resume := dto.JsonResume{
		Basics: &dto.ResumeBasics{
			Name:    strp2("Jane Doe"),
			Label:   strp2("Senior Backend Engineer"),
			Email:   strp2("jane@example.com"),
			Summary: strp2("Experienced backend engineer specializing in distributed systems."),
		},
		Work: []dto.ResumeWork{
			{Name: "Acme Corp", Position: strp2("Senior Engineer"), StartDate: strp2("2020"), EndDate: strp2("2023"),
				Highlights: []string{"Built the thing", "Scaled the other thing"}},
		},
		Skills: []dto.ResumeSkill{{Name: "Backend", Keywords: []string{"Go", "Postgres"}}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path, err := renderer.RenderResume(ctx, resume, "smoke-resume.pdf")
	if err != nil {
		t.Fatalf("RenderResume: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected pdf at %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatal("pdf is empty")
	}
	t.Logf("produced resume PDF: %s (%d bytes)", path, info.Size())

	letterPath, err := renderer.RenderCoverLetter(ctx, "Hello,\n\nI am excited to apply.\n\nBest,\nJane", strp2("Jane Doe"), "Acme Corp", "Senior Engineer", "smoke-cover.pdf")
	if err != nil {
		t.Fatalf("RenderCoverLetter: %v", err)
	}
	info2, err := os.Stat(letterPath)
	if err != nil {
		t.Fatalf("expected pdf at %s: %v", letterPath, err)
	}
	t.Logf("produced cover letter PDF: %s (%d bytes)", letterPath, info2.Size())
}
