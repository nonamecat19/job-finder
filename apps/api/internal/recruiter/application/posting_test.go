package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/recruiter/domain"
)

// fakeExtractor stands in for a ContactExtractor — recruiter's Go LLM path
// is deleted (T113: AIContactExtractor is the only implementation left,
// verified live in t113-parity-samples.md), so what these tests exercise is
// groundContact and the rest of Service's Go-owned grounding logic, fed a
// canned extraction result exactly as AIContactExtractor would return one.
type fakeExtractor struct {
	contacts []ExtractedContact
	err      error
}

func (f *fakeExtractor) Extract(ctx context.Context, source, text string) ([]ExtractedContact, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.contacts, nil
}

// extractPostingContact wires a Service around a fake extractor so existing
// tests can keep calling a plain function rather than going through the
// full Service/Resolve path — T107 moved this extraction behind
// Service.extractor (ContactExtractor) so it can be swapped for the
// `recruiter` capability.
func extractPostingContact(c ExtractedContact, description string) (*domain.ResolvedContact, error) {
	s := &Service{extractor: &fakeExtractor{contacts: []ExtractedContact{c}}}
	return s.extractPostingContact(context.Background(), description)
}

func TestPostingParseNamedContact(t *testing.T) {
	body := "We are hiring a backend engineer.\n\nContact: Jane Doe, Recruiter <jane@acme.com>"
	c := ExtractedContact{Name: "Jane Doe", Title: "Recruiter", Email: "jane@acme.com"}

	contact, err := extractPostingContact(c, body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact == nil {
		t.Fatal("expected a resolved contact, got nil")
	}
	if contact.Name != "Jane Doe" {
		t.Errorf("Name = %q, want %q", contact.Name, "Jane Doe")
	}
	if contact.Title == nil || *contact.Title != "Recruiter" {
		t.Errorf("Title = %v, want Recruiter", contact.Title)
	}
	if contact.Email == nil || *contact.Email != "jane@acme.com" {
		t.Errorf("Email = %v, want jane@acme.com", contact.Email)
	}
	if contact.Source != domain.SourcePosting {
		t.Errorf("Source = %q, want %q", contact.Source, domain.SourcePosting)
	}
	if contact.Confidence < 0.85 {
		t.Errorf("Confidence = %v, want >= 0.85 for an explicit Contact: line", contact.Confidence)
	}
}

func TestPostingNoContact(t *testing.T) {
	body := "We are hiring a backend engineer. Apply through our careers page."
	c := ExtractedContact{}

	contact, err := extractPostingContact(c, body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected nil contact for a posting naming no one, got %+v", contact)
	}
}

func TestPostingGenericMailbox(t *testing.T) {
	body := "Interested candidates should email jobs@acme.com with their resume."
	c := ExtractedContact{Email: "jobs@acme.com"}

	contact, err := extractPostingContact(c, body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected no named contact for a generic mailbox, got %+v", contact)
	}
}

func TestPostingGenericMailboxDefenseInDepth(t *testing.T) {
	body := "Contact: Jane Doe, Recruiter. Applications: jobs@acme.com"
	c := ExtractedContact{Name: "Jane Doe", Title: "Recruiter", Email: "jobs@acme.com"}

	contact, err := extractPostingContact(c, body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact == nil {
		t.Fatal("expected the named contact to survive")
	}
	if contact.Email != nil {
		t.Errorf("expected the generic mailbox to be dropped, got email %v", *contact.Email)
	}
}

func TestPostingFieldTraceability(t *testing.T) {
	body := "Questions about this role? Call +1 555-123-4567 for details."
	c := ExtractedContact{Name: "John Smith", Phone: "+1 555-123-4567"}

	contact, err := extractPostingContact(c, body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected no contact when the only extracted name is not grounded in the source, got %+v", contact)
	}
}

func TestPostingCyrillic(t *testing.T) {
	body := "Вакансія бекенд-розробника.\n\nКонтакт: Ірина Коваленко, Рекрутер <irina@acme.ua>"
	c := ExtractedContact{Name: "Ірина Коваленко", Title: "Рекрутер", Email: "irina@acme.ua"}

	contact, err := extractPostingContact(c, body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact == nil {
		t.Fatal("expected a resolved contact")
	}
	if contact.Name != "Ірина Коваленко" {
		t.Errorf("Name = %q, want byte-identical %q", contact.Name, "Ірина Коваленко")
	}
	if contact.Title == nil || *contact.Title != "Рекрутер" {
		t.Errorf("Title = %v, want byte-identical Рекрутер", contact.Title)
	}
	if contact.Email == nil || *contact.Email != "irina@acme.ua" {
		t.Errorf("Email = %v, want irina@acme.ua", contact.Email)
	}
}

func TestPostingEmptyDescription(t *testing.T) {
	c := ExtractedContact{Name: "should not be called"}
	contact, err := extractPostingContact(c, "   ")
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected nil contact for empty description, got %+v", contact)
	}
}
