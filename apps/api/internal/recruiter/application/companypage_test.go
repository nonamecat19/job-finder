package application

import (
	"context"
	"os"
	"testing"

	"github.com/job-finder/api/internal/recruiter/domain"
)

func extractCompanyPageContacts(contacts []ExtractedContact, text string) ([]domain.ResolvedContact, error) {
	s := &Service{extractor: &fakeExtractor{contacts: contacts}}
	return s.extractCompanyPageContacts(context.Background(), text)
}

func TestCompanyPageParse(t *testing.T) {
	html, err := os.ReadFile("testdata/company_team.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text := extractPageText(string(html))
	if text == "" {
		t.Fatal("expected non-empty flattened text from the fixture")
	}

	extracted := []ExtractedContact{
		{Name: "Jane Doe", Title: "Senior Recruiter", Email: "jane@acme.com"},
		{Name: "Tom Baker", Title: "Head of Talent Acquisition"},
	}

	contacts, err := extractCompanyPageContacts(extracted, text)
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	if len(contacts) < 1 {
		t.Fatalf("expected at least 1 contact from the team fixture, got %d", len(contacts))
	}
	for _, c := range contacts {
		if c.Source != domain.SourceCompanyPage {
			t.Errorf("Source = %q, want %q", c.Source, domain.SourceCompanyPage)
		}
	}
	names := map[string]bool{}
	for _, c := range contacts {
		names[c.Name] = true
	}
	if !names["Jane Doe"] || !names["Tom Baker"] {
		t.Errorf("expected Jane Doe and Tom Baker, got %+v", contacts)
	}
}

func TestCompanyPageParseNoTeamSection(t *testing.T) {
	html, err := os.ReadFile("testdata/company_about_no_team.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text := extractPageText(string(html))

	contacts, err := extractCompanyPageContacts(nil, text)
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected 0 contacts for a page with no People/Team section, got %d", len(contacts))
	}
}

func TestCompanyPageParseUngroundedContactDropped(t *testing.T) {
	extracted := []ExtractedContact{{Name: "Fabricated Person"}}

	contacts, err := extractCompanyPageContacts(extracted, "About Acme Corp. We build software.")
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected the ungrounded contact to be dropped, got %+v", contacts)
	}
}
