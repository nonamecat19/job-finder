package application

import (
	"context"
	"github.com/job-finder/api/internal/recruiter/domain"
	"testing"

	"github.com/job-finder/api/internal/platform/llm"
)

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

// CompleteChat satisfies the 037 Provider interface. The fake's behaviour lives
// in CompleteJSON, so this delegates to it with the final turn as the prompt —
// which is what the real adapters do in reverse. Tool calls are never
// fabricated here: a fake that invented one would make a tool-loop test pass
// for the wrong reason.
func (f *fakeLLM) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	text, err := f.CompleteJSON(ctx, prompt, opts)
	return llm.ChatResult{Content: text}, err
}
func (f *fakeLLM) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }

// 044 T042: recruiter asks the gateway for the `recruiter` task key.
//
// Salary, outreach and recruiter all used to share the `default` group, so a
// change made for one of them silently moved the other two and reporting could
// not tell their spend apart. compose.go now hands each its own router; this
// asserts the key survives the trip through the extractor to the gateway, and
// is paired with the same assertion in internal/salary/application and
// internal/outreach/application — the three keys together are what "distinct"
// means, and each package can only speak for its own.
func TestRecruiterRequestsItsOwnTaskKey(t *testing.T) {
	gw := &taskKeyRecorder{fakeLLM: fakeLLM{
		json: `{"name":"Jane Doe","title":"Recruiter","email":"jane@acme.com","phone":"","linkedInUrl":""}`,
	}}
	body := "We are hiring a backend engineer.\n\nContact: Jane Doe, Recruiter <jane@acme.com>"

	if _, err := ExtractPostingContact(context.Background(), llm.NewRouter("recruiter", gw), "", body); err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}

	if len(gw.keys) == 0 {
		t.Fatal("the gateway was never called, so no task key was requested")
	}
	for _, got := range gw.keys {
		if got != "recruiter" {
			t.Errorf("gateway asked for task key %q, want %q", got, "recruiter")
		}
	}
}

// taskKeyRecorder is the fake gateway a Router talks to, so a test can see the
// task key the router stamped rather than the one it was handed.
type taskKeyRecorder struct {
	fakeLLM
	keys []string
}

func (r *taskKeyRecorder) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	key := ""
	if opts != nil {
		key = opts.TaskKey
	}
	r.keys = append(r.keys, key)
	return r.fakeLLM.CompleteJSON(ctx, prompt, opts)
}

func (r *taskKeyRecorder) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	text, err := r.CompleteJSON(ctx, prompt, opts)
	return llm.ChatResult{Content: text}, err
}

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
	if contact.Confidence < 0.85 {
		t.Errorf("Confidence = %v, want >= 0.85 for an explicit Contact: line", contact.Confidence)
	}
}

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

func TestPostingFieldTraceability(t *testing.T) {
	body := "Questions about this role? Call +1 555-123-4567 for details."
	llmc := &fakeLLM{json: `{"name":"John Smith","title":"","email":"","phone":"+1 555-123-4567","linkedInUrl":""}`}

	contact, err := ExtractPostingContact(context.Background(), llmc, "", body)
	if err != nil {
		t.Fatalf("ExtractPostingContact: %v", err)
	}
	if contact != nil {
		t.Errorf("expected no contact when the only extracted name is not grounded in the source, got %+v", contact)
	}
}

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
