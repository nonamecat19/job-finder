package application

import (
	"context"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/llm"
)

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

func strPtr(s string) *string { return &s }

func sampleIntel() *dto.CompanyIntelDto {
	return &dto.CompanyIntelDto{
		CompanyName: "Acme Corp",
		Funding:     strPtr("$50M Series B in 2024"),
		TechStack:   strPtr("Go, React, Postgres"),
	}
}

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

// 044 T042: outreach asks the gateway for the `outreach` task key.
//
// Salary, outreach and recruiter all used to share the `default` group, so a
// change made for one of them silently moved the other two and reporting could
// not tell their spend apart. compose.go now hands each its own router; this
// asserts the key survives the trip through the service to the gateway, and is
// paired with the same assertion in internal/salary/application and
// internal/recruiter/application — the three keys together are what "distinct"
// means, and each package can only speak for its own.
func TestOutreachRequestsItsOwnTaskKey(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	gw := &taskKeyRecorder{fakeLLM: fakeLLM{responses: []string{
		`{"text":"Hi Jane, saw you're using Go, React, Postgres.","specificClaims":["Go, React, Postgres"]}`,
	}}}
	svc := NewService(contacts, &fakeIntel{intel: sampleIntel()}, llm.NewRouter("outreach", gw), "")

	if _, err := svc.GenerateDraft(context.Background(), "job-1", "c1", "warm"); err != nil {
		t.Fatalf("GenerateDraft: %v", err)
	}

	if len(gw.keys) == 0 {
		t.Fatal("the gateway was never called, so no task key was requested")
	}
	for _, got := range gw.keys {
		if got != "outreach" {
			t.Errorf("gateway asked for task key %q, want %q", got, "outreach")
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

func TestGenerateDraft_OverLengthRetriesThenFits(t *testing.T) {
	tooLong := strings.Repeat("word ", 200)
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

func TestGenerateDraft_FabricatedClaimRetriedThenFixed(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	intel := &fakeIntel{intel: sampleIntel()}
	llmc := &fakeLLM{responses: []string{
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

func TestGenerateDraft_UnknownContactID(t *testing.T) {
	contacts := &fakeContacts{contacts: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe"}}}
	svc := NewService(contacts, &fakeIntel{}, &fakeLLM{}, "")

	_, err := svc.GenerateDraft(context.Background(), "job-1", "does-not-exist", "warm")
	if err != ErrContactNotFound {
		t.Errorf("err = %v, want ErrContactNotFound", err)
	}
}

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
