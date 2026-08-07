package adapters

import (
	"context"
	"fmt"
	"strconv"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/roster"
)

type greenhouseJob struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	AbsURL   string `json:"absolute_url"`
	UpdateAt string `json:"updated_at"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
}

type greenhouseResponse struct {
	Jobs []greenhouseJob `json:"jobs"`
}

type GreenhouseAdapter struct {
	Roster *roster.Service
	boardRunState
}

func (a *GreenhouseAdapter) Key() string          { return "greenhouse" }
func (a *GreenhouseAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func (a *GreenhouseAdapter) Search(ctx context.Context, _ dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	return runBoardVendor(ctx, a.Roster, &a.boardRunState, "greenhouse", a.fetchEmployer)
}

func (a *GreenhouseAdapter) HealthCheck(ctx context.Context, _ map[string]any) (bool, error) {
	return vendorHealthCheck(ctx, a.Roster, "greenhouse", a.fetchEmployer)
}

func (a *GreenhouseAdapter) fetchEmployer(ctx context.Context, employer sqlcgen.EmployerBoard) (int, []dto.NormalizedJob, error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", employer.EmployerIdentifier)
	var res greenhouseResponse
	status, err := jobsources.GetJSONStatus(ctx, nil, url, nil, &res)
	if err != nil {
		return status, nil, err
	}
	jobs := make([]dto.NormalizedJob, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   a.Key(),
			ExternalID:  jobsources.Ptr(strconv.FormatInt(j.ID, 10)),
			Title:       j.Title,
			Company:     employer.DisplayName,
			Location:    jobsources.NilIfEmpty(j.Location.Name),
			Remote:      remoteWordRe.MatchString(j.Location.Name) || remoteWordRe.MatchString(j.Title),
			URL:         j.AbsURL,
			Description: j.Content,
			PostedAt:    jobsources.NilIfEmpty(j.UpdateAt),
			Raw:         j,
		})
	}
	return status, jobs, nil
}
