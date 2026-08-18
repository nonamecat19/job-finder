package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/aiclient"
)

const (
	SourcePosting     = "posting"
	SourceCompanyPage = "company_page"
	SourceLinkedIn    = "linkedin"
)

type ExtractedContact struct {
	Name        string
	Title       string
	Email       string
	Phone       string
	LinkedInURL string
}

type ContactExtractor interface {
	Extract(ctx context.Context, source, text string) ([]ExtractedContact, error)
}

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
