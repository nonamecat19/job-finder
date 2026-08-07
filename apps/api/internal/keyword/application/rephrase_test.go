package application

import (
	"bytes"
	"context"
	"errors"
	"github.com/job-finder/api/internal/keyword/domain"
	"log/slog"
	"strings"
	"testing"
)

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

type fixedFinder struct {
	bullet string
	ok     bool
}

func (f fixedFinder) FindEvidence(domain.DiffTerm, []string) (string, bool) { return f.bullet, f.ok }

func reqTerm(term, canonical string) domain.DiffTerm {
	return domain.DiffTerm{Term: term, Canonical: canonical, Polarity: domain.PolarityRequired}
}

func bufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestSuggest_HonestRephrase(t *testing.T) {
	model := &fakeModel{outputs: []string{
		"Containerized services with Docker and orchestrated them using Kubernetes across staging.",
	}}
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

func TestSuggest_RejectsInventedTechnology(t *testing.T) {
	model := &fakeModel{outputs: []string{
		"Rewrote the payment service in Rust for performance.",
		"Built high-performance services in Rust and Go.",
	}}
	bullet := "Built high-performance backend services in Go."
	log, buf := bufLogger()
	s := NewSuggester(model).
		WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true}).
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
	if !strings.Contains(buf.String(), "rejected rephrase generation") {
		t.Errorf("expected rejection to be logged, log was:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "Rust") {
		t.Errorf("expected violating entity in log, log was:\n%s", buf.String())
	}
}

func TestSuggest_NoEvidence_NoModelCall(t *testing.T) {
	model := &fakeModel{outputs: []string{"should never be used"}}
	s := NewSuggester(model)

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

func TestSuggest_RejectsInventedMetric(t *testing.T) {
	model := &fakeModel{outputs: []string{
		"Optimized the API and improved throughput by 40%.",
		"Optimized the API and improved throughput significantly.",
	}}
	bullet := "Optimized the API to reduce latency under load."
	s := NewSuggester(model).WithEvidenceFinder(fixedFinder{bullet: bullet, ok: true})

	got := s.Suggest(context.Background(), reqTerm("API", "API"), []string{bullet})

	if got.Rephrase == nil {
		t.Fatal("expected honest retry to succeed")
	}
	if strings.Contains(*got.Rephrase, "40%") {
		t.Errorf("emitted rephrase still contains invented metric: %q", *got.Rephrase)
	}
	if model.calls != 2 {
		t.Errorf("expected 2 attempts (reject then accept), got %d", model.calls)
	}
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

func TestSuggestAll_Alignment(t *testing.T) {
	model := &fakeModel{outputs: []string{"Deployed apps on Kubernetes clusters."}}
	profile := []string{"Ran workloads on Kubernetes in production."}
	s := NewSuggester(model)

	terms := []domain.DiffTerm{
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
