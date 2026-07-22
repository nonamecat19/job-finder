package outreach

import (
	"context"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/llm"
)

// fakeContacts implements ContactsProvider with a fixed response, mirroring
// the fakeLLM pattern used across the repo's other service tests.
type fakeContacts struct {
	contacts []dto.JobContactDto
	err      error
}

func (f *fakeContacts) ListContacts(ctx context.Context, jobID string) ([]dto.JobContactDto, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.contacts, nil
}

// fakeIntel implements IntelProvider with a fixed response.
type fakeIntel struct {
	intel *dto.CompanyIntelDto
	err   error
}

func (f *fakeIntel) GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.intel, nil
}

// fakeLLM implements llm.Provider and returns a queue of fixed JSON
// responses from CompleteJSON, one per call, repeating the last once
// exhausted — lets a test script a "fabricate then correct" retry sequence.
type fakeLLM struct {
	responses []string
	calls     int
}

func (f *fakeLLM) ModelName() string { return "test-model" }
func (f *fakeLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	idx := f.calls
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	f.calls++
	if idx < 0 {
		return "", nil
	}
	return f.responses[idx], nil
}
func (f *fakeLLM) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }

func strPtr(s string) *string { return &s }

func sampleIntel() *dto.CompanyIntelDto {
	return &dto.CompanyIntelDto{
		CompanyName: "Acme Corp",
		Funding:     strPtr("$50M Series B in 2024"),
		TechStack:   strPtr("Go, React, Postgres"),
	}
}

// TestGenerateDraft_AddressesRealContact covers US1 AS1: the draft names
// the resolved contact, not an invented one.
func TestGenerateDraft_AddressesRealContact(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{
		{ID: "c1", Name: "Jane Doe", Source: "posting", Confidence: 0.9},
	}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{
		`{"text":"Hi Jane, I noticed you use Go, React, Postgres — would love to connect.","specificClaims":["Go, React, Postgres"]}`,
	}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if out.ContactID == nil || *out.ContactID != "c1" {
		t.Errorf("ContactID = %v, want c1", out.ContactID)
	}
	if out.ContactName == nil || *out.ContactName != "Jane Doe" {
		t.Errorf("ContactName = %v, want Jane Doe", out.ContactName)
	}
	if !strings.Contains(out.Text, "Jane") {
		t.Errorf("draft text does not address the contact: %q", out.Text)
	}
}

// TestGenerateDraft_ClaimsTraceToSignals covers US1 AS3 / US3 AS1: every
// grounding trace's claim is a substring of the message and of the signal
// value it cites.
func TestGenerateDraft_ClaimsTraceToSignals(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{
		`{"text":"Hi Jane, saw you're using Go, React, Postgres — impressive stack.","specificClaims":["Go, React, Postgres"]}`,
	}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if len(out.GroundingTraces) != 1 {
		t.Fatalf("expected 1 grounding trace, got %d: %+v", len(out.GroundingTraces), out.GroundingTraces)
	}
	tr := out.GroundingTraces[0]
	if !strings.Contains(strings.ToLower(out.Text), strings.ToLower(tr.Claim)) {
		t.Errorf("claim %q not found verbatim in text %q", tr.Claim, out.Text)
	}
	if tr.SignalKind != "tech_stack" {
		t.Errorf("SignalKind = %q, want tech_stack", tr.SignalKind)
	}
}

// TestGenerateDraft_NoSignals_GenericOpener covers the "no company-intel
// signals" edge case (FR-012, SC-003): the draft is a generic, honest
// opener with zero specific claims, and the LLM is never even called.
func TestGenerateDraft_NoSignals_GenericOpener(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: nil}
	llmc := &fakeLLM{responses: []string{`not valid json at all`}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if len(out.GroundingTraces) != 0 {
		t.Errorf("expected zero grounding traces with no signals, got %+v", out.GroundingTraces)
	}
	if out.Text == "" {
		t.Error("expected a non-empty generic opener")
	}
	if llmc.calls != 0 {
		t.Errorf("expected zero LLM calls when no signals exist, got %d", llmc.calls)
	}
}

// TestGenerateDraft_ToneChangesWordingNotFacts covers US2 AS2 / SC-006:
// two tones draw the same grounded fact but read differently.
func TestGenerateDraft_ToneChangesWordingNotFacts(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}

	warmLLM := &fakeLLM{responses: []string{
		`{"text":"Hi Jane! Loved seeing Go, React, Postgres in the stack — would love to chat!","specificClaims":["Go, React, Postgres"]}`,
	}}
	formalLLM := &fakeLLM{responses: []string{
		`{"text":"Dear Jane Doe, I understand your team relies on Go, React, Postgres. I would welcome a discussion.","specificClaims":["Go, React, Postgres"]}`,
	}}

	warmOut, err := NewService(contacts, intel, warmLLM, "").GenerateDraft(context.Background(), "job-1", "c1", "warm")
	if err != nil {
		t.Fatalf("warm GenerateDraft: %v", err)
	}
	formalOut, err := NewService(contacts, intel, formalLLM, "").GenerateDraft(context.Background(), "job-1", "c1", "formal")
	if err != nil {
		t.Fatalf("formal GenerateDraft: %v", err)
	}

	if warmOut.Text == formalOut.Text {
		t.Error("expected different wording between tones")
	}
	if len(warmOut.GroundingTraces) != 1 || len(formalOut.GroundingTraces) != 1 {
		t.Fatalf("expected 1 trace each, got warm=%d formal=%d", len(warmOut.GroundingTraces), len(formalOut.GroundingTraces))
	}
	if warmOut.GroundingTraces[0].SignalKind != formalOut.GroundingTraces[0].SignalKind ||
		warmOut.GroundingTraces[0].SignalValue != formalOut.GroundingTraces[0].SignalValue {
		t.Errorf("expected identical grounded fact across tones, got warm=%+v formal=%+v",
			warmOut.GroundingTraces[0], formalOut.GroundingTraces[0])
	}
}

// TestGenerateDraft_DefaultTone covers FR-011: an empty/unknown tone
// defaults rather than erroring.
func TestGenerateDraft_DefaultTone(t *testing.T) {
	contacts := &fakeContacts{}
	intel := &fakeIntel{}
	svc := NewService(contacts, intel, &fakeLLM{}, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "", "")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if out.Tone != string(DefaultTone) {
		t.Errorf("Tone = %q, want default %q", out.Tone, DefaultTone)
	}

	out2, err := svc.GenerateDraft(context.Background(), "job-1", "", "not-a-real-tone")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if out2.Tone != string(DefaultTone) {
		t.Errorf("Tone = %q for unknown input, want default %q", out2.Tone, DefaultTone)
	}
}

// TestGenerateDraft_OverLengthRetriesThenFits covers FR-009: an over-length
// first attempt is rejected and retried rather than ever presented.
func TestGenerateDraft_OverLengthRetriesThenFits(t *testing.T) {
	tooLong := strings.Repeat("word ", 200) // way over maxDraftChars
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{
		`{"text":"` + tooLong + `","specificClaims":[]}`,
		`{"text":"Hi Jane, short and grounded in Go, React, Postgres.","specificClaims":["Go, React, Postgres"]}`,
	}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "direct")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if len(out.Text) > maxDraftChars {
		t.Errorf("text length %d exceeds limit %d", len(out.Text), maxDraftChars)
	}
	if llmc.calls < 2 {
		t.Errorf("expected a retry after the over-length attempt, got %d calls", llmc.calls)
	}
}

// TestGenerateDraft_AllAttemptsOverLength_FallsBackGeneric covers FR-009's
// "never presented" guarantee even when every attempt violates the limit.
func TestGenerateDraft_AllAttemptsOverLength_FallsBackGeneric(t *testing.T) {
	tooLong := strings.Repeat("word ", 200)
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{`{"text":"` + tooLong + `","specificClaims":[]}`}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if len(out.Text) > maxDraftChars {
		t.Fatalf("text length %d exceeds limit %d", len(out.Text), maxDraftChars)
	}
	if len(out.GroundingTraces) != 0 {
		t.Errorf("expected the safe generic fallback (no traces), got %+v", out.GroundingTraces)
	}
}

// TestGenerateDraft_FabricatedClaimRetriedThenFixed covers FR-006: a claim
// that isn't a verbatim substring of any allowed fact is never presented —
// the first (fabricated) attempt is discarded, not stripped-and-kept.
func TestGenerateDraft_FabricatedClaimRetriedThenFixed(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{
		// Fabricates a Series C round that was never a stored signal.
		`{"text":"Hi Jane, congrats on the Series C round!","specificClaims":["Series C round"]}`,
		`{"text":"Hi Jane, saw your Go, React, Postgres stack — nice.","specificClaims":["Go, React, Postgres"]}`,
	}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if strings.Contains(out.Text, "Series C") {
		t.Errorf("fabricated claim leaked into presented draft: %q", out.Text)
	}
	for _, tr := range out.GroundingTraces {
		if tr.Claim == "Series C round" {
			t.Errorf("fabricated claim was traced as grounded: %+v", tr)
		}
	}
	if llmc.calls < 2 {
		t.Errorf("expected a retry after the fabricated claim, got %d calls", llmc.calls)
	}
}

// TestGenerateDraft_AllAttemptsFabricate_FallsBackGeneric covers FR-005/
// FR-012: when grounded generation can never be produced, the draft is the
// honest generic opener, never a fabricated one.
func TestGenerateDraft_AllAttemptsFabricate_FallsBackGeneric(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{
		`{"text":"Hi Jane, congrats on your Series C!","specificClaims":["Series C"]}`,
	}}
	svc := NewService(contacts, intel, llmc, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if strings.Contains(out.Text, "Series C") {
		t.Errorf("fabricated claim leaked into fallback draft: %q", out.Text)
	}
	if len(out.GroundingTraces) != 0 {
		t.Errorf("expected zero traces in the fallback, got %+v", out.GroundingTraces)
	}
	if llmc.calls != maxAttempts {
		t.Errorf("expected exactly %d attempts before falling back, got %d", maxAttempts, llmc.calls)
	}
}

// TestGenerateDraft_MultipleContacts_RequiresChoice covers FR-008 / AS9:
// two resolved contacts must never be silently merged or guessed among.
func TestGenerateDraft_MultipleContacts_RequiresChoice(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{
		{ID: "c1", Name: "Jane Doe"},
		{ID: "c2", Name: "Bob Recruiter"},
	}}
	svc := NewService(contacts, &fakeIntel{}, &fakeLLM{}, "")

	_, err := svc.GenerateDraft(context.Background(), "job-1", "", "warm")
	if err != ErrContactRequired {
		t.Errorf("err = %v, want ErrContactRequired", err)
	}
}

// TestGenerateDraft_UnknownContactID covers FR-007: a caller-supplied
// contactId that doesn't match a resolved contact is an error, never a
// silently invented recipient.
func TestGenerateDraft_UnknownContactID(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	svc := NewService(contacts, &fakeIntel{}, &fakeLLM{}, "")

	_, err := svc.GenerateDraft(context.Background(), "job-1", "does-not-exist", "warm")
	if err != ErrContactNotFound {
		t.Errorf("err = %v, want ErrContactNotFound", err)
	}
}

// TestGenerateDraft_NoContact_NeutralSalutation covers the "no resolved
// contact" edge case: the system never guesses a name.
func TestGenerateDraft_NoContact_NeutralSalutation(t *testing.T) {
	svc := NewService(&fakeContacts{}, &fakeIntel{}, &fakeLLM{}, "")

	out, err := svc.GenerateDraft(context.Background(), "job-1", "", "warm")
	if err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}
	if out.ContactID != nil || out.ContactName != nil {
		t.Errorf("expected no contact, got id=%v name=%v", out.ContactID, out.ContactName)
	}
	if out.Text == "" {
		t.Error("expected a non-empty neutral-salutation draft")
	}
}

// TestTones covers FR-010/FR-011: a defined set of tones with exactly one
// marked default.
func TestTones(t *testing.T) {
	svc := NewService(&fakeContacts{}, &fakeIntel{}, &fakeLLM{}, "")
	tones := svc.Tones()
	if len(tones) != 3 {
		t.Fatalf("expected 3 tones, got %d", len(tones))
	}
	defaults := 0
	for _, to := range tones {
		if to.Default {
			defaults++
			if to.Value != string(DefaultTone) {
				t.Errorf("default tone value = %q, want %q", to.Value, DefaultTone)
			}
		}
	}
	if defaults != 1 {
		t.Errorf("expected exactly 1 default tone, got %d", defaults)
	}
}
