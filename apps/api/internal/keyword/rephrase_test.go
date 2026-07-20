package keyword

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// fakeModel is a deterministic RephraseModel. It returns queued outputs in
// order (one per attempt) and records how many times it was called and the
// prompts it saw, so tests can assert the no-model-call path and retry feed.
type fakeModel struct {
	outputs []string
	err     error
	calls   int
	prompts []string
}

func (f *fakeModel) Rephrase(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		return "", f.err
	}
	i := f.calls
	f.calls++
	if i < len(f.outputs) {
		return f.outputs[i], nil
	}
	if len(f.outputs) > 0 {
		return f.outputs[len(f.outputs)-1], nil
	}
	return "", nil
}

// fixedFinder forces a specific evidence outcome, isolating grounding tests
// from the lexical finder's heuristics.
type fixedFinder struct {
	bullet string
	ok     bool
}

func (f fixedFinder) FindEvidence(DiffTerm, []string) (string, bool) { return f.bullet, f.ok }

func reqTerm(term, canonical string) DiffTerm {
	return DiffTerm{Term: term, Canonical: canonical, Polarity: PolarityRequired}
}

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// --- happy path -----------------------------------------------------------

func TestSuggest_HonestRephrase(t *testing.T) {
	model := &fakeModel{outputs: []string{
		"Containerized services with Docker and orchestrated them using Kubernetes across staging.",
	}}
	// Source bullet already mentions Docker & Kubernetes; the rephrase only
	// reframes existing experience, so grounding passes.
	bullet := "Built and deployed Docker containers managed by Kubernetes in production."
	s := NewSuggester(model).WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true})

	got := s.Suggest(context.Background(), reqTerm("Kubernetes", "Kubernetes"), []string{bullet})

	if got.Reason != "" {
		t.Fatalf("expected honest rephrase, got reason %q", got.Reason)
	}
	if got.Rephrase == nil {
		t.Fatal("expected non-nil rephrase")
	}
	if got.SourceBullet != bullet {
		t.Errorf("source bullet not traced: %q", got.SourceBullet)
	}
	if model.calls != 1 {
		t.Errorf("expected 1 model call, got %d", model.calls)
	}
}

// --- adversarial: invented technology --------------------------------------

func TestSuggest_RejectsInventedTechnology(t *testing.T) {
	// JD demands Rust; the profile has no Rust. The model is tempted to invent
	// it in every attempt. Both attempts must be rejected by grounding, and the
	// no-honest-rephrase path must fire.
	model := &fakeModel{outputs: []string{
		"Rewrote the payment service in Rust for performance.",
		"Built high-performance services in Rust and Go.",
	}}
	bullet := "Built high-performance backend services in Go."
	log, buf := bufLogger()
	s := NewSuggester(model).
		WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true}). // force past evidence gate
		WithLogger(log)

	got := s.Suggest(context.Background(), reqTerm("Rust", "Rust"), []string{bullet})

	if got.Rephrase != nil {
		t.Fatalf("expected no rephrase (invented tech), got %q", *got.Rephrase)
	}
	if got.Reason != ReasonNoHonestRephrase {
		t.Fatalf("expected reason %q, got %q", ReasonNoHonestRephrase, got.Reason)
	}
	if model.calls != rephraseAttempts {
		t.Errorf("expected %d attempts, got %d", rephraseAttempts, model.calls)
	}
	// Rejected generations must be logged (acceptance criterion).
	if !strings.Contains(buf.String(), "rejected rephrase generation") {
		t.Errorf("expected rejection to be logged, log was:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "Rust") {
		t.Errorf("expected violating entity in log, log was:\n%s", buf.String())
	}
}

// --- adversarial: JD term absent + no evidence -----------------------------

func TestSuggest_NoEvidence_NoModelCall(t *testing.T) {
	// The profile shares no vocabulary with the term, so the lexical finder
	// reports no evidence. The suggester must NOT call the model and must
	// return the explicit no-rephrase result rather than a soft suggestion.
	model := &fakeModel{outputs: []string{"should never be used"}}
	s := NewSuggester(model) // default lexical finder

	profile := []string{"Managed a team of five and shipped a billing dashboard."}
	got := s.Suggest(context.Background(), reqTerm("Rust", "Rust"), profile)

	if got.Reason != ReasonNoHonestRephrase {
		t.Fatalf("expected %q, got reason %q", ReasonNoHonestRephrase, got.Reason)
	}
	if got.Rephrase != nil {
		t.Fatalf("expected nil rephrase, got %q", *got.Rephrase)
	}
	if model.calls != 0 {
		t.Errorf("model must not be called without evidence, calls=%d", model.calls)
	}
}

// --- adversarial: fabricated metric ----------------------------------------

func TestSuggest_RejectsInventedMetric(t *testing.T) {
	// Source bullet has no numbers; the model fabricates "40%". Grounding must
	// reject it.
	model := &fakeModel{outputs: []string{
		"Optimized the API and improved throughput by 40%.",
		"Optimized the API and improved throughput significantly.", // honest retry
	}}
	bullet := "Optimized the API to reduce latency under load."
	s := NewSuggester(model).WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true})

	got := s.Suggest(context.Background(), reqTerm("API", "API"), []string{bullet})

	// The second (honest) attempt has no invented number and should be emitted.
	if got.Rephrase == nil {
		t.Fatal("expected honest retry to succeed")
	}
	if strings.Contains(*got.Rephrase, "40%") {
		t.Errorf("emitted rephrase still contains invented metric: %q", *got.Rephrase)
	}
	if model.calls != 2 {
		t.Errorf("expected 2 attempts (reject then accept), got %d", model.calls)
	}
	// The retry prompt must feed the prior violation back.
	if len(model.prompts) < 2 || !strings.Contains(model.prompts[1], "previous attempt violated") {
		t.Errorf("retry prompt did not feed violation back")
	}
}

func TestSuggest_AllowsMetricPresentInSource(t *testing.T) {
	model := &fakeModel{outputs: []string{"Cut API latency by 30% during peak load."}}
	bullet := "Reduced API latency by 30% under peak traffic."
	s := NewSuggester(model).WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true})

	got := s.Suggest(context.Background(), reqTerm("API", "API"), []string{bullet})
	if got.Rephrase == nil {
		t.Fatalf("metric present in source should be allowed, got reason %q", got.Reason)
	}
}

// --- model error is not a suggestion ---------------------------------------

func TestSuggest_ModelError_NoSuggestion(t *testing.T) {
	model := &fakeModel{err: errors.New("boom")}
	bullet := "Built services in Go."
	log, buf := bufLogger()
	s := NewSuggester(model).
		WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true}).
		WithLogger(log)

	got := s.Suggest(context.Background(), reqTerm("Go", "Go"), []string{bullet})
	if got.Reason != ReasonNoHonestRephrase || got.Rephrase != nil {
		t.Fatalf("model error must yield no-rephrase, got %+v", got)
	}
	if !strings.Contains(buf.String(), "rephrase model error") {
		t.Errorf("model error should be logged, log:\n%s", buf.String())
	}
}

// --- SuggestAll aligns 1:1 -------------------------------------------------

func TestSuggestAll_Alignment(t *testing.T) {
	// One term with evidence, one without. The result must be aligned 1:1.
	model := &fakeModel{outputs: []string{"Deployed apps on Kubernetes clusters."}}
	profile := []string{"Ran workloads on Kubernetes in production."}
	s := NewSuggester(model) // lexical finder: "Kubernetes" overlaps, "Rust" does not

	terms := []DiffTerm{
		reqTerm("Kubernetes", "Kubernetes"),
		reqTerm("Rust", "Rust"),
	}
	got := s.SuggestAll(context.Background(), terms, profile)
	if len(got) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(got))
	}
	if got[0].Rephrase == nil {
		t.Errorf("term with evidence should get a rephrase")
	}
	if got[1].Reason != ReasonNoHonestRephrase {
		t.Errorf("term without evidence should get no-rephrase, got %q", got[1].Reason)
	}
}

// --- grounding unit ---------------------------------------------------------

func TestVerifyRephraseGrounding(t *testing.T) {
	bullet := "Reduced API latency by 30% using Go."
	term := reqTerm("API", "API")
	allowed := properNounSet([]string{bullet})
	_ = term
	nums := numberSet(bullet)

	tests := []struct {
		name     string
		rephrase string
		wantViol bool
	}{
		{"grounded reword", "Cut API latency 30% with Go.", false},
		{"invented tech", "Rebuilt the API in Rust.", true},
		{"invented metric", "Cut API latency by 90%.", true},
		{"empty", "   ", true},
		{"lowercase glue ok", "improved the api latency by 30% in go", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := verifyRephraseGrounding(bullet, allowed, nums, tc.rephrase)
			if tc.wantViol && len(v) == 0 {
				t.Errorf("expected violation, got none for %q", tc.rephrase)
			}
			if !tc.wantViol && len(v) != 0 {
				t.Errorf("unexpected violations %v for %q", v, tc.rephrase)
			}
		})
	}
}

func TestLexicalEvidenceFinder(t *testing.T) {
	finder := LexicalEvidenceFinder{}
	profile := []string{"Built UIs with React and Redux."}

	if _, ok := finder.FindEvidence(reqTerm("React Native", "React Native"), profile); !ok {
		t.Error("expected adjacency via shared 'React' token")
	}
	if _, ok := finder.FindEvidence(reqTerm("Rust", "Rust"), profile); ok {
		t.Error("expected no adjacency for unrelated term")
	}
}
