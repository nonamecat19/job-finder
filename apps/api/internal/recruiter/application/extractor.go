package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/aiclient"
)

// Capability source discriminators (recruiter.py's Source enum) — distinct
// from domain.Source* (the stored contact's provenance label, which uses a
// hyphen for company-page): "posting" | "company_page" | "linkedin".
const (
	SourcePosting     = "posting"
	SourceCompanyPage = "company_page"
	SourceLinkedIn    = "linkedin"
)

// ExtractedContact is the capability-agnostic shape every ContactExtractor
// implementation returns — the same fields extractedContact/
// extractedContactList carried before T107, now shared by both the legacy
// gateway path and the `recruiter` capability path so groundContact (field-
// level grounding, which stays in Go per recruiter.py's docstring) works
// identically either way.
type ExtractedContact struct {
	Name        string
	Title       string
	Email       string
	Phone       string
	LinkedInURL string
}

// ContactExtractor runs one of the three recruiter extraction sources
// (posting/company_page/linkedin) and returns every contact the model
// reported, ungrounded — the caller (posting.go/companypage.go/linkedin.go)
// still runs groundContact on each result.
type ContactExtractor interface {
	Extract(ctx context.Context, source, text string) ([]ExtractedContact, error)
}

// AIContactExtractor calls the `recruiter` capability over aiclient
// (contracts/http.md H1-1) — the capability builds its own source-specific
// prompt server-side (prompts/recruiter.py); this just sends source/text
// and parses the result.
type AIContactExtractor struct {
	Client *aiclient.Client
}

var _ ContactExtractor = (*AIContactExtractor)(nil)

type recruiterInput struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

type recruiterContact struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	LinkedInURL string `json:"linkedin_url"`
}

type recruiterResult struct {
	Contacts []recruiterContact `json:"contacts"`
}

func (e *AIContactExtractor) Extract(ctx context.Context, source, text string) ([]ExtractedContact, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	resp, err := e.Client.Invoke(ctx, "recruiter", recruiterInput{Source: source, Text: text}, aiclient.RequestContext{})
	if err != nil {
		return nil, fmt.Errorf("recruiter: %s extraction: %w", source, err)
	}
	if resp.Failure != nil {
		return nil, fmt.Errorf("recruiter: %s extraction failed: %s", source, resp.Failure.Message)
	}

	var out recruiterResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return nil, fmt.Errorf("recruiter: %s extraction: unmarshal result: %w", source, err)
	}
	results := make([]ExtractedContact, 0, len(out.Contacts))
	for _, c := range out.Contacts {
		results = append(results, ExtractedContact(c))
	}
	return results, nil
}
