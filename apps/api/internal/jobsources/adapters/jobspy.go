package adapters

import (
	"context"
	"os"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
)

type jobspySidecarJob struct {
	ID          *string `json:"id"`
	Site        string  `json:"site"`
	Title       string  `json:"title"`
	Company     *string `json:"company"`
	Location    *string `json:"location"`
	IsRemote    *bool   `json:"is_remote"`
	Salary      *string `json:"salary"`
	JobURL      string  `json:"job_url"`
	Description *string `json:"description"`
	DatePosted  *string `json:"date_posted"`
}

type jobspySearchRequest struct {
	Site          string `json:"site"`
	SearchTerm    string `json:"search_term"`
	Location      string `json:"location,omitempty"`
	IsRemote      bool   `json:"is_remote"`
	ResultsWanted int    `json:"results_wanted"`
}

type jobspySearchResponse struct {
	Jobs []jobspySidecarJob `json:"jobs"`
}

type jobspyHealthResponse struct {
	OK bool `json:"ok"`
}

// JobSpyAdapter delegates LinkedIn/Indeed/Glassdoor to the python jobspy
// sidecar (apps/jobspy-sidecar). Best-effort: upstream scrapers break
// periodically. Mirrors jobspy.adapter.ts.
type JobSpyAdapter struct{}

func (JobSpyAdapter) Key() string          { return "jobspy" }
func (JobSpyAdapter) Kind() dto.SourceKind { return dto.SourceKindSidecar }

func (JobSpyAdapter) baseURL(config map[string]any) string {
	return jobsources.StringOr(config["url"], envOr("JOBSPY_URL", "http://jobspy-sidecar:8000"))
}

func (a JobSpyAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	site := "linkedin"
	if query.Site != nil && *query.Site != "" {
		site = *query.Site
	} else if s, ok := config["site"].(string); ok && s != "" {
		site = s
	}
	location := ""
	if query.Location != nil {
		location = *query.Location
	}
	remote := false
	if query.Remote != nil {
		remote = *query.Remote
	}

	var res jobspySearchResponse
	err := jobsources.PostJSON(ctx, nil, a.baseURL(config)+"/search", jobspySearchRequest{
		Site:          site,
		SearchTerm:    query.Keywords,
		Location:      location,
		IsRemote:      remote,
		ResultsWanted: 30,
	}, &res, 120*time.Second)
	if err != nil {
		return nil, err
	}

	jobs := make([]dto.NormalizedJob, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		company := "Unknown"
		if j.Company != nil && *j.Company != "" {
			company = *j.Company
		}
		remoteVal := false
		if j.IsRemote != nil {
			remoteVal = *j.IsRemote
		}
		description := ""
		if j.Description != nil {
			description = *j.Description
		}
		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "jobspy",
			ExternalID:  j.ID,
			Title:       j.Title,
			Company:     company,
			Location:    j.Location,
			Remote:      remoteVal,
			SalaryRaw:   j.Salary,
			URL:         j.JobURL,
			Description: description,
			PostedAt:    j.DatePosted,
			Raw:         j,
		})
	}
	return jobs, nil
}

func (a JobSpyAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	var res jobspyHealthResponse
	if err := jobsources.GetJSON(ctx, nil, a.baseURL(config)+"/health", nil, &res); err != nil {
		return false, nil
	}
	return res.OK, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
