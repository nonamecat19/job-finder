package domain_test

import (
	"reflect"
	"testing"

	"github.com/job-finder/api/internal/keyword/domain"
)

func term(raw string, polarity domain.Polarity) domain.ExtractedTerm {
	res, _ := domain.NewExtractor().Extract("Requirements\n- " + raw)
	for _, t := range res.Terms {
		t.Polarity = polarity
		return t
	}
	return domain.ExtractedTerm{}
}

func canonicals(terms []domain.DiffTerm) []string {
	out := make([]string, len(terms))
	for i, t := range terms {
		out[i] = t.Canonical
	}
	return out
}

func findDiff(terms []domain.DiffTerm, canonical string) (domain.DiffTerm, bool) {
	for _, t := range terms {
		if t.Canonical == canonical {
			return t, true
		}
	}
	return domain.DiffTerm{}, false
}

func TestExactMatch(t *testing.T) {
	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("Kubernetes", domain.PolarityRequired),
	}}
	res := domain.NewDiffer().Diff(jd, []string{"Kubernetes"})

	got, ok := findDiff(res.Matched, "Kubernetes")
	if !ok {
		t.Fatalf("Kubernetes should be matched, got matched=%v", canonicals(res.Matched))
	}
	if got.MatchType != domain.MatchExact {
		t.Errorf("expected exact match, got %q", got.MatchType)
	}
	if len(res.MissingRequired) != 0 {
		t.Errorf("expected no missing-required, got %v", canonicals(res.MissingRequired))
	}
}

func TestSynonymMatchIsExact(t *testing.T) {
	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("K8s", domain.PolarityRequired),
	}}
	res := domain.NewDiffer().Diff(jd, []string{"Kubernetes"})

	got, ok := findDiff(res.Matched, "Kubernetes")
	if !ok {
		t.Fatalf("K8s should match Kubernetes via synonym, got matched=%v", canonicals(res.Matched))
	}
	if got.MatchType != domain.MatchExact {
		t.Errorf("expected exact (canonical) match, got %q", got.MatchType)
	}
}

func TestNormalizedMatch(t *testing.T) {
	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("Microservices", domain.PolarityRequired),
	}}
	res := domain.NewDiffer().Diff(jd, []string{"microservice"})

	got, ok := findDiff(res.Matched, jd.Terms[0].Canonical)
	if !ok {
		t.Fatalf("Microservices should normalize-match microservice, matched=%v missingReq=%v",
			canonicals(res.Matched), canonicals(res.MissingRequired))
	}
	if got.MatchType != domain.MatchNormalized {
		t.Errorf("expected normalized match, got %q", got.MatchType)
	}
}

func TestNoMatchRequiredAndPreferred(t *testing.T) {
	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("Rust", domain.PolarityRequired),
		term("Haskell", domain.PolarityPreferred),
	}}
	res := domain.NewDiffer().Diff(jd, []string{"Python", "Go"})

	if _, ok := findDiff(res.MissingRequired, "Rust"); !ok {
		t.Errorf("Rust should be missing-required, got %v", canonicals(res.MissingRequired))
	}
	if _, ok := findDiff(res.MissingPreferred, "Haskell"); !ok {
		t.Errorf("Haskell should be missing-preferred, got %v", canonicals(res.MissingPreferred))
	}
	if len(res.Matched) != 0 {
		t.Errorf("expected nothing matched, got %v", canonicals(res.Matched))
	}
}

func TestCoveragePct(t *testing.T) {
	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("Python", domain.PolarityRequired),
		term("Go", domain.PolarityRequired),
		term("Rust", domain.PolarityRequired),
		term("gRPC", domain.PolarityPreferred),
	}}
	res := domain.NewDiffer().Diff(jd, []string{"Python", "Go"})

	if res.Metadata.TotalRequired != 3 || res.Metadata.TotalPreferred != 1 {
		t.Fatalf("totals wrong: %+v", res.Metadata)
	}
	if res.Metadata.MatchedRequired != 2 {
		t.Fatalf("matchedRequired want 2, got %d", res.Metadata.MatchedRequired)
	}
	if want := 50.0; res.Metadata.CoveragePct != want {
		t.Errorf("coveragePct want %.1f, got %.1f", want, res.Metadata.CoveragePct)
	}
}

func TestDeterministicOrdering(t *testing.T) {
	mk := func() *domain.ExtractResult {
		return &domain.ExtractResult{Terms: []domain.ExtractedTerm{
			term("Zsh", domain.PolarityRequired),
			term("Ansible", domain.PolarityRequired),
			term("Terraform", domain.PolarityPreferred),
			term("Docker", domain.PolarityRequired),
		}}
	}
	shuffled := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		mk().Terms[2], mk().Terms[0], mk().Terms[3], mk().Terms[1],
	}}

	d := domain.NewDiffer()
	a := d.Diff(mk(), nil)
	b := d.Diff(shuffled, nil)

	if !reflect.DeepEqual(a, b) {
		t.Fatalf("ordering not deterministic:\n a=%v\n b=%v",
			canonicals(a.MissingRequired), canonicals(b.MissingRequired))
	}
	got := canonicals(a.MissingRequired)
	want := []string{"Ansible", "Docker", "Zsh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missing-required order want %v, got %v", want, got)
	}
}

func TestExtractResumeTerms(t *testing.T) {
	profile := `
Skills: Python, Kubernetes, Terraform
Certifications: AWS Certified Solutions Architect
`
	terms := domain.ExtractResumeTerms(profile)
	set := map[string]bool{}
	for _, s := range terms {
		set[s] = true
	}
	for _, want := range []string{"Python", "Kubernetes", "Terraform", "AWS"} {
		if !set[want] {
			t.Errorf("expected %q in resume terms, got %v", want, terms)
		}
	}

	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("Kubernetes", domain.PolarityRequired),
		term("Rust", domain.PolarityRequired),
	}}
	res := domain.NewDiffer().Diff(jd, terms)
	if _, ok := findDiff(res.Matched, "Kubernetes"); !ok {
		t.Errorf("Kubernetes from profile should match, matched=%v", canonicals(res.Matched))
	}
	if _, ok := findDiff(res.MissingRequired, "Rust"); !ok {
		t.Errorf("Rust should be missing, got %v", canonicals(res.MissingRequired))
	}
}
