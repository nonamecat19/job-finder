package adapters

import (
	"context"
	"fmt"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/roster"
)

type ashbyJob struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Location        string `json:"location"`
	IsRemote        bool   `json:"isRemote"`
	JobURL          string `json:"jobUrl"`
	DescriptionHTML string `json:"descriptionHtml"`
	PublishedAt     string `json:"publishedAt"`
}

type ashbyResponse struct {
	Jobs []ashbyJob `json:"jobs"`
}

type AshbyAdapter struct {
	Roster *roster.Service
	boardRunState
}

func (a *AshbyAdapter) Key() string          { return "ashby" }
func (a *AshbyAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func (a *AshbyAdapter) Search(ctx context.Context, _ dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	return runBoardVendor(ctx, a.Roster, &a.boardRunState, "ashby", a.fetchEmployer)
}

func (a *AshbyAdapter) HealthCheck(ctx context.Context, _ map[string]any) (bool, error) {
	return vendorHealthCheck(ctx, a.Roster, "ashby", a.fetchEmployer)
}

func (a *AshbyAdapter) fetchEmployer(ctx context.Context, employer sqlcgen.EmployerBoard) (int, []dto.NormalizedJob, error) {
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", employer.EmployerIdentifier)
	var res ashbyResponse
	status, err := jobsources.GetJSONStatus(ctx, nil, url, nil, &res)
	if err != nil {
		return status, nil, err
	}
	jobs := make([]dto.NormalizedJob, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   a.Key(),
			ExternalID:  jobsources.Ptr(j.ID),
			Title:       j.Title,
			Company:     employer.DisplayName,
			Location:    jobsources.NilIfEmpty(j.Location),
			Remote:      j.IsRemote || remoteWordRe.MatchString(j.Location),
			URL:         j.JobURL,
			Description: j.DescriptionHTML,
			PostedAt:    jobsources.NilIfEmpty(j.PublishedAt),
			Raw:         j,
		})
	}
	return status, jobs, nil
}
