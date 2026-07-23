package companyintel

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/scraping"
)

const builtwithDomain = "builtwith.com"

// knownTechKeywords is the fallback keyword list used to infer a tech
// stack straight from a job posting's description when BuiltWith is
// unreachable (FR-015). Not exhaustive — a curated set of common
// languages, frameworks, datastores, and infra tools.
var knownTechKeywords = []string{
	"React", "Angular", "Vue", "Svelte", "Next.js", "Nuxt",
	"TypeScript", "JavaScript", "Python", "Go", "Golang", "Java", "Kotlin",
	"Ruby", "Rails", "PHP", "Laravel", "C#", ".NET", "C++", "Rust", "Scala",
	"Node.js", "Express", "Django", "Flask", "FastAPI", "Spring", "Spring Boot",
	"PostgreSQL", "MySQL", "MongoDB", "Redis", "Elasticsearch", "DynamoDB",
	"Cassandra", "SQLite", "GraphQL", "REST", "gRPC",
	"AWS", "GCP", "Azure", "Docker", "Kubernetes", "Terraform", "Ansible",
	"Jenkins", "GitLab CI", "GitHub Actions", "CircleCI",
	"Kafka", "RabbitMQ", "Airflow", "Spark", "Hadoop",
	"TensorFlow", "PyTorch", "scikit-learn",
	"Tailwind", "Tailwind CSS", "Bootstrap", "SASS",
	"Swift", "SwiftUI", "Objective-C", "Flutter", "React Native",
}

var techKeywordRes = compileTechKeywordRes(knownTechKeywords)

func compileTechKeywordRes(keywords []string) map[string]*regexp.Regexp {
	out := make(map[string]*regexp.Regexp, len(keywords))
	for _, kw := range keywords {
		escaped := regexp.QuoteMeta(kw)
		out[kw] = regexp.MustCompile(`(?i)(^|[^a-z0-9])` + escaped + `($|[^a-z0-9])`)
	}
	return out
}

// TechStackScraper reads a BuiltWith public lookup page for the company's
// detected technologies, falling back to keyword-matching the job
// posting's own description when BuiltWith is unreachable or empty
// (FR-015). If both sources are empty, the signal is omitted entirely.
type TechStackScraper struct {
	Scraping *scraping.Service
}

func (TechStackScraper) Kind() string   { return KindTechStack }
func (TechStackScraper) Domain() string { return builtwithDomain }

func (s TechStackScraper) Scrape(ctx context.Context, in Input) (*SignalResult, error) {
	if result := s.tryBuiltWith(ctx, in.Website); result != nil {
		return result, nil
	}

	if techs := matchKnownTech(in.JobDescription); len(techs) > 0 {
		return &SignalResult{
			Kind:   KindTechStack,
			Value:  strings.Join(techs, ", "),
			Source: "job-posting-description",
		}, nil
	}

	return nil, fmt.Errorf("techstack: no BuiltWith result and no keywords matched in job description")
}

// tryBuiltWith attempts the primary source and returns nil (not an error)
// on any failure — the caller falls through to the description keyword
// fallback, matching FR-015 exactly.
func (s TechStackScraper) tryBuiltWith(ctx context.Context, website string) *SignalResult {
	domain := hostOf(website)
	if domain == "" {
		return nil
	}
	lookupURL := fmt.Sprintf("https://builtwith.com/%s", domain)

	html, err := s.Scraping.FetchHTML(ctx, lookupURL, nil)
	if err != nil {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	return parseBuiltWith(doc, lookupURL)
}

func hostOf(website string) string {
	if website == "" {
		return ""
	}
	u, err := url.Parse(website)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// parseBuiltWith collects every `.tech-item` label on the lookup page.
// Returns nil (no error) when the page has no matches — the caller treats
// that identically to a fetch failure and falls back to keyword matching.
func parseBuiltWith(doc *goquery.Document, sourceURL string) *SignalResult {
	var techs []string
	doc.Find(".tech-item").Each(func(_ int, sel *goquery.Selection) {
		if t := strings.TrimSpace(sel.Text()); t != "" {
			techs = append(techs, t)
		}
	})
	if len(techs) == 0 {
		return nil
	}
	return &SignalResult{
		Kind:   KindTechStack,
		Value:  strings.Join(techs, ", "),
		Source: sourceURL,
	}
}

// matchKnownTech scans description against knownTechKeywords, returning
// matches in the list's canonical order and casing, deduplicated.
func matchKnownTech(description string) []string {
	if description == "" {
		return nil
	}
	var out []string
	for _, kw := range knownTechKeywords {
		if techKeywordRes[kw].MatchString(description) {
			out = append(out, kw)
		}
	}
	return out
}
