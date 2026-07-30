package application

import (
	"context"
	"github.com/job-finder/api/internal/recruiter/domain"
	"testing"

	"github.com/job-finder/api/internal/llm"
)

// fakeLLM implements llm.Provider and returns a fixed JSON response from
// CompleteJSON, mirroring the fakeLLM pattern in internal/salary's tests.
type fakeLLM struct {
	json string
	err  error
}

func (f *fakeLLM) ModelName() string { return "test-model" }
func (f *fakeLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.json, nil
}
func (f *fakeLLM) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }

// TestPostingParseNamedContact covers SC-001: an explicit "Contact:" line
// with a name, title, and email yields a grounded posting contact.
func TestPostingParseNamedContact(t *testing.T) {
	body := "We are hiring a backend engineer.\n\nContact: Jane Doe, Recruiter <jane@acme.com>"
	llmc := &fakeLLM{json: `{"name":"Jane Doe","title":"Recruiter","email":"jane@acme.com","phone":"","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
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
	// Explicit "Contact:" label -> top confidence tier.
	if contact.Confidence < 0.85 {
		t.Errorf("Confidence = %v, want >= 0.85 for an explicit Contact: line", contact.Confidence)
	}
}

// TestPostingNoContact covers SC-002: a posting naming no one yields zero
// rows and no error.
func TestPostingNoContact(t *testing.T) {
	body := "We are hiring a backend engineer. Apply through our careers page."
	llmc := &fakeLLM{json: `{"name":"","title":"","email":"","phone":"","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected nil contact for a posting naming no one, got %+v", contact)
	}
}

// TestPostingGenericMailbox covers FR-007/SC-003: a generic mailbox with no
// personal name never surfaces as a named human, even if the LLM tries to
// report the address.
func TestPostingGenericMailbox(t *testing.T) {
	body := "Interested candidates should email jobs@acme.com with their resume."
	llmc := &fakeLLM{json: `{"name":"","title":"","email":"jobs@acme.com","phone":"","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected no named contact for a generic mailbox, got %+v", contact)
	}
}

// TestPostingGenericMailboxDefenseInDepth exercises the generic-mailbox
// filter directly: even if a name IS grounded, a generic-role email must
// never be reported as that person's personal channel.
func TestPostingGenericMailboxDefenseInDepth(t *testing.T) {
	body := "Contact: Jane Doe, Recruiter. Applications: jobs@acme.com"
	llmc := &fakeLLM{json: `{"name":"Jane Doe","title":"Recruiter","email":"jobs@acme.com","phone":"","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
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

// TestPostingFieldTraceability is the non-negotiable Principle II gate: a
// body with a phone but no name must not yield a name-bearing row, and no
// field may be reported that is absent from (or hallucinated beyond) the
// input text.
func TestPostingFieldTraceability(t *testing.T) {
	body := "Questions about this role? Call +1 555-123-4567 for details."
	// Adversarial: the LLM hallucinates a name that never appears in body.
	llmc := &fakeLLM{json: `{"name":"John Smith","title":"","email":"","phone":"+1 555-123-4567","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected no contact when the only extracted name is not grounded in the source, got %+v", contact)
	}
}

// TestPostingCyrillic covers non-ASCII round-tripping: a Cyrillic contact
// line's name/title come back byte-identical to the source text.
func TestPostingCyrillic(t *testing.T) {
	body := "Вакансія бекенд-розробника.\n\nКонтакт: Ірина Коваленко, Рекрутер <irina@acme.ua>"
	llmc := &fakeLLM{json: `{"name":"Ірина Коваленко","title":"Рекрутер","email":"irina@acme.ua","phone":"","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
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

// TestPostingEmptyDescription covers the degenerate empty-input case: no
// LLM call is even needed, and the result is nil, nil.
func TestPostingEmptyDescription(t *testing.T) {
	llmc := &fakeLLM{json: `{"name":"should not be called"}`}
	contact, err := ExtractPostingContact(context.Background(), llmc, "", "   ")
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected nil contact for empty description, got %+v", contact)
	}
}
