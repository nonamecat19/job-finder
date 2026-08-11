package adapters

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/companyintel/domain"
	"github.com/nonamecat19/jobscraper/scraping"
)

const builtwithDomain = "builtwith.com"

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

type TechStackScraper struct {
	Scraping scraping.Scraper
}

func (TechStackScraper) Kind() string   { return domain.KindTechStack }
func (TechStackScraper) Domain() string { return builtwithDomain }

func (s TechStackScraper) Scrape(ctx context.Context, in domain.Input) (*domain.SignalResult, error) {
	if result := s.tryBuiltWith(ctx, in.Website); result != nil {
		return result, nil
	}

	if techs := matchKnownTech(in.JobDescription); len(techs) > 0 {
		return &domain.SignalResult{
			Kind:   domain.KindTechStack,
			Value:  strings.Join(techs, ", "),
			Source: "job-posting-description",
		}, nil
	}

	return nil, fmt.Errorf("techstack: no BuiltWith result and no keywords matched in job description")
}

func (s TechStackScraper) tryBuiltWith(ctx context.Context, website string) *domain.SignalResult {
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

func parseBuiltWith(doc *goquery.Document, sourceURL string) *domain.SignalResult {
	var techs []string
	doc.Find(".tech-item").Each(func(_ int, sel *goquery.Selection) {
		if t := strings.TrimSpace(sel.Text()); t != "" {
			techs = append(techs, t)
		}
	})
	if len(techs) == 0 {
		return nil
	}
	return &domain.SignalResult{
		Kind:   domain.KindTechStack,
		Value:  strings.Join(techs, ", "),
		Source: sourceURL,
	}
}

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
