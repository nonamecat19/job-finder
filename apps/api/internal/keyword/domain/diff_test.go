package domain_test

import (
	"reflect"
	"testing"

	"github.com/job-finder/api/internal/keyword/domain"
)

// term is a tiny constructor for an already-normalized JD term. We call the
// package's extractor path for realism where it matters, but for the pure diff
// tests we build terms directly so each case isolates one match path.
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

// TestExactMatch: a resume term whose canonical form equals the JD term's
// canonical lands in matched with MatchExact.
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

// TestSynonymMatchIsExact: JD uses an acronym/alias, resume uses the full form.
// Synonym resolution collapses both to the same canonical → exact match.
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

// TestNormalizedMatch: canonical strings differ but stemming makes them equal
// (inflectional variation) → matched with MatchNormalized.
func TestNormalizedMatch(t *testing.T) {
	jd := &domain.ExtractResult{Terms: []domain.ExtractedTerm{
		term("Microservices", domain.PolarityRequired),
	}}
	// "microservices" stems (plural strip) to "microservice", matching the
	// singular resume term even though the canonical strings differ.
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

// TestNoMatchRequiredAndPreferred: unmatched terms fall into the correct
// missing bucket by polarity.
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

// TestCoveragePct: coverage is (matchedRequired+matchedPreferred)/total*100.
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

// TestDeterministicOrdering: buckets are sorted so repeated runs and shuffled
// input produce byte-identical output (UI stability acceptance criterion).
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
	// Missing-required must be canonical-sorted.
	got := canonicals(a.MissingRequired)
	want := []string{"Ansible", "Docker", "Zsh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missing-required order want %v, got %v", want, got)
	}
}

// TestExtractResumeTerms pulls terms from whole-profile free text so the diff
// sees adjacent evidence beyond the rendered resume (009 requirement).
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

	// End-to-end: whole-profile terms drive the diff.
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
