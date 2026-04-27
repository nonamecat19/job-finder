// Package adapters implements one JobSourceAdapter per job board, mirroring
// modules/job-sources/adapters/*.adapter.ts.
package adapters

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/jobsources"
)

var remoteWordRe = regexp.MustCompile(`(?i)remote`)

type adzunaResult struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Company struct {
		DisplayName string `json:"display_name"`
	} `json:"company"`
	Location struct {
		DisplayName string `json:"display_name"`
	} `json:"location"`
	RedirectURL string  `json:"redirect_url"`
	Description string  `json:"description"`
	SalaryMin   float64 `json:"salary_min"`
	SalaryMax   float64 `json:"salary_max"`
	Created     string  `json:"created"`
}

type adzunaResponse struct {
	Results []adzunaResult `json:"results"`
}

// AdzunaAdapter mirrors adzuna.adapter.ts.
type AdzunaAdapter struct{}

func (AdzunaAdapter) Key() string          { return "adzuna" }
func (AdzunaAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func (a AdzunaAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	appID := jobsources.StringOr(config["appId"], os.Getenv("ADZUNA_APP_ID"))
	appKey := jobsources.StringOr(config["appKey"], os.Getenv("ADZUNA_APP_KEY"))
	if appID == "" || appKey == "" {
		return nil, fmt.Errorf("adzuna: appId/appKey not configured")
	}
	country := ""
	if query.Country != nil {
		country = *query.Country
	}
	if country == "" {
		country = jobsources.StringOr(config["country"], os.Getenv("ADZUNA_COUNTRY"))
	}
	if country == "" {
		country = "gb"
	}

	params := url.Values{}
	params.Set("app_id", appID)
	params.Set("app_key", appKey)
	params.Set("what", query.Keywords)
	if query.Location != nil {
		params.Set("where", *query.Location)
	}
	if query.SalaryMin != nil {
		params.Set("salary_min", strconv.FormatFloat(*query.SalaryMin, 'f', -1, 64))
	}
	params.Set("results_per_page", "50")
	params.Set("content-type", "application/json")

	apiURL := fmt.Sprintf("https://api.adzuna.com/v1/api/jobs/%s/search/1", strings.ToLower(country))
	var res adzunaResponse
	if err := jobsources.GetJSON(ctx, nil, apiURL, params, &res); err != nil {
		return nil, fmt.Errorf("adzuna: %w", err)
	}

	jobs := make([]dto.NormalizedJob, 0, len(res.Results))
	for _, r := range res.Results {
		company := r.Company.DisplayName
		if company == "" {
			company = "Unknown"
		}
		var salaryRaw *string
		if r.SalaryMin != 0 || r.SalaryMax != 0 {
			s := fmt.Sprintf("%s – %s", numOrQ(r.SalaryMin), numOrQ(r.SalaryMax))
			salaryRaw = &s
		}
		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "adzuna",
			ExternalID:  jobsources.Ptr(r.ID),
			Title:       r.Title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(r.Location.DisplayName),
			Remote:      remoteWordRe.MatchString(r.Title + " " + r.Description),
			SalaryRaw:   salaryRaw,
			URL:         r.RedirectURL,
			Description: r.Description,
			PostedAt:    jobsources.NilIfEmpty(r.Created),
			Raw:         r,
		})
	}
	return jobs, nil
}

func (a AdzunaAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	_, err := a.Search(ctx, dto.SearchQuery{Keywords: "developer"}, config)
	return err == nil, nil
}

func numOrQ(n float64) string {
	if n == 0 {
		return "?"
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}
