package application

import (
	"context"
	"github.com/job-finder/api/internal/recruiter/domain"
	"os"
	"testing"
)

func TestCompanyPageParse(t *testing.T) {
	html, err := os.ReadFile("testdata/company_team.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text := extractPageText(string(html))
	if text == "" {
		t.Fatal("expected non-empty flattened text from the fixture")
	}

	llmc := &fakeLLM{json: `{"contacts":[
		{"name":"Jane Doe","title":"Senior Recruiter","email":"jane@acme.com","phone":"","linkedInUrl":""},
		{"name":"Tom Baker","title":"Head of Talent Acquisition","email":"","phone":"","linkedInUrl":""}
	]}`}

	contacts, err := ExtractCompanyPageContacts(context.Background(), llmc, "", text)
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

	llmc := &fakeLLM{json: `{"contacts":[]}`}

	contacts, err := ExtractCompanyPageContacts(context.Background(), llmc, "", text)
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected 0 contacts for a page with no People/Team section, got %d", len(contacts))
	}
}

func TestCompanyPageParseUngroundedContactDropped(t *testing.T) {
	llmc := &fakeLLM{json: `{"contacts":[{"name":"Fabricated Person","title":"","email":"","phone":"","linkedInUrl":""}]}`}

	contacts, err := ExtractCompanyPageContacts(context.Background(), llmc, "", "About Acme Corp. We build software.")
	if err != nil {
		t.Fatalf("ExtractCompanyPageContacts: %v", err)
	}
	if len(contacts) != 0 {
		t.Errorf("expected the ungrounded contact to be dropped, got %+v", contacts)
	}
}
