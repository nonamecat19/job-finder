package adapters

// Fixtures under testdata/ are hand-authored, minimal reproductions of the
// selectors each parser targets (crunchbase_org.html, layoffs_fyi.html,
// glassdoor_reviews.html, company_about.html, builtwith_lookup.html, plus
// each source's degraded-input sibling) — no live network access is used
// in this package's tests.

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/companyintel/domain"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func docFromFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(loadFixture(t, name)))
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return doc
}

func TestParseCrunchbaseFunding_LatestRound(t *testing.T) {
	doc := docFromFixture(t, "crunchbase_org.html")
	result, err := parseCrunchbaseFunding(doc, "https://www.crunchbase.com/organization/acme-corp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Series B — $50M (Jan 15, 2025)"
	if result.Value != want {
		t.Errorf("Value = %q, want %q", result.Value, want)
	}
	if result.Kind != domain.KindFunding {
		t.Errorf("Kind = %q, want %q", result.Kind, domain.KindFunding)
	}
}

func TestParseCrunchbaseFunding_NoRounds(t *testing.T) {
	doc := docFromFixture(t, "crunchbase_empty.html")
	_, err := parseCrunchbaseFunding(doc, "https://www.crunchbase.com/organization/unknown")
	if err == nil {
		t.Fatal("expected an error for a page with no funding rounds")
	}
}

func TestCrunchbaseSlug(t *testing.T) {
	cases := map[string]string{
		"Acme Corp":      "acme-corp",
		"  Acme, Inc.  ": "acme-inc",
		"already-a-slug": "already-a-slug",
	}
	for in, want := range cases {
		if got := crunchbaseSlug(in); got != want {
			t.Errorf("crunchbaseSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLayoffsRow_Match(t *testing.T) {
	doc := docFromFixture(t, "layoffs_fyi.html")
	result := parseLayoffsRow(doc, "Acme Corp", "https://layoffs.fyi/")
	if result == nil {
		t.Fatal("expected a result")
	}
	if result.Kind != domain.KindLayoffs {
		t.Errorf("Kind = %q, want %q", result.Kind, domain.KindLayoffs)
	}
	want := "120 laid off (15%) on 2025-03-01"
	if result.Value != want {
		t.Errorf("Value = %q, want %q", result.Value, want)
	}
}

func TestParseLayoffsRow_NoMatch_IsZeroResultNotError(t *testing.T) {
	doc := docFromFixture(t, "layoffs_fyi.html")
	result := parseLayoffsRow(doc, "Never Heard Of This Co", "https://layoffs.fyi/")
	if result == nil {
		t.Fatal("expected a zero-result success, not nil")
	}
	if result.Value != noLayoffData {
		t.Errorf("Value = %v, want %q", result.Value, noLayoffData)
	}
}

func TestParseLayoffsRow_CaseInsensitiveMatch(t *testing.T) {
	doc := docFromFixture(t, "layoffs_fyi.html")
	result := parseLayoffsRow(doc, "acme corp", "https://layoffs.fyi/")
	if result.Value == noLayoffData {
		t.Fatal("expected a case-insensitive match, got zero-result")
	}
}

func TestParseGlassdoorRating_Found(t *testing.T) {
	doc := docFromFixture(t, "glassdoor_reviews.html")
	result, err := parseGlassdoorRating(doc, "https://www.glassdoor.com/Reviews/acme-corp-reviews-SRCH_KE0,9.htm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rating, ok := result.Value.(float64)
	if !ok || rating != 3.8 {
		t.Errorf("Value = %v, want 3.8", result.Value)
	}
}

func TestParseGlassdoorRating_Blocked(t *testing.T) {
	doc := docFromFixture(t, "glassdoor_blocked.html")
	_, err := parseGlassdoorRating(doc, "https://www.glassdoor.com/Reviews/acme-corp-reviews-SRCH_KE0,9.htm")
	if err == nil {
		t.Fatal("expected an error when the rating cannot be found (blocked/challenged page)")
	}
}

func TestParseHeadcount_Baseline(t *testing.T) {
	doc := docFromFixture(t, "company_about.html")
	result, err := parseHeadcount(doc, nil, "https://acme.example.com/about")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "350 employees (baseline captured)"
	if result.Value != want {
		t.Errorf("Value = %q, want %q", result.Value, want)
	}
}

func TestParseHeadcount_TrendUp(t *testing.T) {
	doc := docFromFixture(t, "company_about.html")
	prev := 300
	result, err := parseHeadcount(doc, &prev, "https://acme.example.com/about")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "350 employees (was 300, up 50)"
	if result.Value != want {
		t.Errorf("Value = %q, want %q", result.Value, want)
	}
}

func TestParseHeadcount_TrendDown(t *testing.T) {
	doc := docFromFixture(t, "company_about.html")
	prev := 400
	result, err := parseHeadcount(doc, &prev, "https://acme.example.com/about")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "350 employees (was 400, down 50)"
	if result.Value != want {
		t.Errorf("Value = %q, want %q", result.Value, want)
	}
}

func TestParseHeadcount_NoFigureFound(t *testing.T) {
	doc := docFromFixture(t, "company_about_no_headcount.html")
	_, err := parseHeadcount(doc, nil, "https://acme.example.com/about")
	if err == nil {
		t.Fatal("expected an error when no employee count is present on the page")
	}
}

func TestParseBuiltWith_Found(t *testing.T) {
	doc := docFromFixture(t, "builtwith_lookup.html")
	result := parseBuiltWith(doc, "https://builtwith.com/acme.example.com")
	if result == nil {
		t.Fatal("expected a result")
	}
	want := "React, TypeScript, Go"
	if result.Value != want {
		t.Errorf("Value = %q, want %q", result.Value, want)
	}
}

func TestParseBuiltWith_Empty(t *testing.T) {
	doc := docFromFixture(t, "builtwith_empty.html")
	result := parseBuiltWith(doc, "https://builtwith.com/unknown.example.com")
	if result != nil {
		t.Errorf("expected nil for an empty technologies list, got %+v", result)
	}
}

func TestMatchKnownTech(t *testing.T) {
	desc := "We use React, TypeScript and Go daily, deployed on AWS with Docker and Kubernetes."
	got := matchKnownTech(desc)
	want := []string{"React", "TypeScript", "Go", "AWS", "Docker", "Kubernetes"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMatchKnownTech_NoMatches(t *testing.T) {
	got := matchKnownTech("We are looking for a great cook to join our kitchen team.")
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestMatchKnownTech_EmptyDescription(t *testing.T) {
	if got := matchKnownTech(""); got != nil {
		t.Errorf("expected nil for empty description, got %v", got)
	}
}

func TestScraperKindsAndDomains(t *testing.T) {
	scrapers := []domain.Scraper{
		CrunchbaseScraper{},
		LayoffsScraper{},
		GlassdoorScraper{},
		HeadcountScraper{},
		TechStackScraper{},
	}
	wantKinds := map[string]string{
		"CrunchbaseScraper": domain.KindFunding,
		"LayoffsScraper":    domain.KindLayoffs,
		"GlassdoorScraper":  domain.KindGlassdoorRating,
		"HeadcountScraper":  domain.KindHeadcount,
		"TechStackScraper":  domain.KindTechStack,
	}
	for _, s := range scrapers {
		if s.Domain() == "" {
			t.Errorf("%T.Domain() is empty", s)
		}
		if s.Kind() == "" {
			t.Errorf("%T.Kind() is empty", s)
		}
	}
	_ = wantKinds
}

func TestRegistryByDomain(t *testing.T) {
	reg := domain.NewRegistry(
		CrunchbaseScraper{},
		LayoffsScraper{},
		GlassdoorScraper{},
	)
	groups := reg.ByDomain()
	if len(groups) != 3 {
		t.Fatalf("expected 3 distinct domains, got %d", len(groups))
	}
	if len(reg.All()) != 3 {
		t.Fatalf("expected 3 scrapers, got %d", len(reg.All()))
	}
}
