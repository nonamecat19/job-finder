package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeProvider lets tests script a sequence of CompleteJSON responses,
// exactly like stubbing cerebras.provider.ts in a Jest test.
type fakeProvider struct {
	responses []string
	calls     int
	prompts   []string
}

func (f *fakeProvider) ModelName() string { return "fake" }
func (f *fakeProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.calls >= len(f.responses) {
		return "", errors.New("no more scripted responses")
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}
func (f *fakeProvider) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }

type fitResult struct {
	Score   int      `json:"score"`
	Summary string   `json:"summary"`
	Skills  []string `json:"skills"`
}

func (f *fitResult) Validate() error {
	if f.Score < 0 || f.Score > 100 {
		return errors.New("score must be between 0 and 100")
	}
	return nil
}

func TestCompleteStructured_MalformedThenValid(t *testing.T) {
	p := &fakeProvider{responses: []string{
		"not json at all",
		"```json\n{\"score\": 85, \"summary\": \"solid fit\", \"skills\": [\"go\"]}\n```",
	}}
	got, err := CompleteStructured[fitResult](context.Background(), p, "rate this candidate", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Score != 85 || got.Summary != "solid fit" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if p.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", p.calls)
	}
	// second prompt must include the "previous answer was invalid" retry hint
	if !strings.Contains(p.prompts[1], "previous answer was invalid") {
		t.Fatalf("retry prompt missing validation-error hint: %s", p.prompts[1])
	}
}

func TestCompleteStructured_FailsAfterMaxRetries(t *testing.T) {
	p := &fakeProvider{responses: []string{"nope", "still nope", "nope again"}}
	_, err := CompleteStructured[fitResult](context.Background(), p, "rate this candidate", nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if p.calls != 3 { // 1 initial + 2 retries, matching structuredRetries=2
		t.Fatalf("expected 3 attempts, got %d", p.calls)
	}
}

func TestCompleteStructured_SemanticValidationRetries(t *testing.T) {
	// First response is valid JSON but fails the Validate() semantic check
	// (score out of range) — must retry, same as zod's schema.safeParse failing.
	p := &fakeProvider{responses: []string{
		`{"score": 150, "summary": "x", "skills": []}`,
		`{"score": 50, "summary": "x", "skills": []}`,
	}}
	got, err := CompleteStructured[fitResult](context.Background(), p, "rate", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Score != 50 {
		t.Fatalf("expected corrected score 50, got %d", got.Score)
	}
}

func TestStripFences(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":1}\n```":     `{"a":1}`,
		`{"a":1}`:                 `{"a":1}`,
	}
	for in, want := range cases {
		if got := stripFences(in); got != want {
			t.Errorf("stripFences(%q) = %q, want %q", in, got, want)
		}
	}
}
