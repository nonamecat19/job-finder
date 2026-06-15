package adapters

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
)

type arbeitnowJob struct {
	Slug        string   `json:"slug"`
	CompanyName string   `json:"company_name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Remote      bool     `json:"remote"`
	URL         string   `json:"url"`
	Tags        []string `json:"tags"`
	JobTypes    []string `json:"job_types"`
	Location    string   `json:"location"`
	CreatedAt   int64    `json:"created_at"`
}

type arbeitnowResponse struct {
	Data  []arbeitnowJob `json:"data"`
	Links struct {
		Next *string `json:"next"`
	} `json:"links"`
}

// ArbeitnowAdapter mirrors arbeitnow.adapter.ts. The API has no keyword
// param, so we fetch the first few pages and filter locally.
type ArbeitnowAdapter struct{}

func (ArbeitnowAdapter) Key() string          { return "arbeitnow" }
func (ArbeitnowAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func (ArbeitnowAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	var collected []arbeitnowJob
	for page := 1; page <= 3; page++ {
		params := url.Values{"page": {strconv.Itoa(page)}}
		var res arbeitnowResponse
		if err := jobsources.GetJSON(ctx, nil, "https://api.arbeitnow.com/api/job-board-api", params, &res); err != nil {
			return nil, err
		}
		collected = append(collected, res.Data...)
		if res.Links.Next == nil {
			break
		}
	}

	terms := strings.Fields(strings.ToLower(query.Keywords))
	jobs := make([]dto.NormalizedJob, 0, len(collected))
	for _, j := range collected {
		haystack := strings.ToLower(j.Title + " " + strings.Join(j.Tags, " ") + " " + j.Description)
		matched := len(terms) == 0
		for _, t := range terms {
			if strings.Contains(haystack, t) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		var postedAt *string
		if j.CreatedAt != 0 {
			postedAt = jobsources.Ptr(time.Unix(j.CreatedAt, 0).UTC().Format(time.RFC3339))
		}
		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "arbeitnow",
			ExternalID:  jobsources.Ptr(j.Slug),
			Title:       j.Title,
			Company:     j.CompanyName,
			Location:    jobsources.NilIfEmpty(j.Location),
			Remote:      j.Remote,
			URL:         j.URL,
			Description: jobsources.HTMLToText(j.Description),
			PostedAt:    postedAt,
			Raw:         j,
		})
	}
	return jobs, nil
}

func (ArbeitnowAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	var res arbeitnowResponse
	if err := jobsources.GetJSON(ctx, nil, "https://api.arbeitnow.com/api/job-board-api", url.Values{"page": {"1"}}, &res); err != nil {
		return false, nil
	}
	return res.Data != nil, nil
}
