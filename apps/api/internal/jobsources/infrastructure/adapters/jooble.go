package adapters

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
)

type joobleJob struct {
	Title       string `json:"title"`
	CompanyName string `json:"companyName"`
	Location    string `json:"location"`
	Snippet     string `json:"snippet"`
	URL         string `json:"url"`
	Salary      string `json:"salary"`
	Date        string `json:"date"`
	Source      string `json:"source"`
}

type joobleResponse struct {
	TotalCount string      `json:"totalCount"`
	Jobs       []joobleJob `json:"jobs"`
}

type JoobleAdapter struct {
	APIKey string
}

func (JoobleAdapter) Key() string          { return "jooble" }
func (JoobleAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func keywordsFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	if path == "" {
		return ""
	}
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return path
	}
	return decoded
}

func (a JoobleAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	apiKey := jobsources.StringOr(config["apiKey"], a.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("jooble: apiKey not configured")
	}

	keywords := query.Keywords
	if query.SubscriptionURL != "" {
		if k := keywordsFromURL(query.SubscriptionURL); k != "" {
			keywords = k
		}
	}

	body := map[string]any{
		"keywords": keywords,
		"page":     "1",
	}
	if query.Location != nil && *query.Location != "" {
		body["location"] = *query.Location
	}

	var res joobleResponse
	apiURL := fmt.Sprintf("https://jooble.org/api/%s", apiKey)
	if err := jobsources.PostJSON(ctx, nil, apiURL, body, &res, 0); err != nil {
		return nil, fmt.Errorf("jooble: %w", err)
	}

	jobs := make([]dto.NormalizedJob, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		company := j.CompanyName
		if company == "" {
			company = "Unknown"
		}
		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "jooble",
			Title:       j.Title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(j.Location),
			Remote:      remoteWordRe.MatchString(j.Title + " " + j.Snippet),
			SalaryRaw:   jobsources.NilIfEmpty(j.Salary),
			URL:         j.URL,
			Description: jobsources.HTMLToText(j.Snippet),
			PostedAt:    jobsources.NilIfEmpty(j.Date),
			Raw:         j,
		})
	}
	return jobs, nil
}

func (a JoobleAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	_, err := a.Search(ctx, dto.SearchQuery{Keywords: "developer"}, config)
	return err == nil, nil
}
