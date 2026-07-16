package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
)

const (
	robotaAPIBase          = "https://api.rabota.ua"
	robotaMaxPages         = 10
	robotaVacanciesPerPage = 59
)

type robotaVacancy struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	CompanyName      string `json:"companyName"`
	CityName         string `json:"cityName"`
	Date             string `json:"date"`
	ShortDescription string `json:"shortDescription"`
	SalaryFrom       uint32 `json:"salaryFrom"`
	SalaryTo         uint32 `json:"salaryTo"`
	SalaryComment    string `json:"salaryComment"`
	NotebookID       uint64 `json:"notebookId"`
	Hot              bool   `json:"hot"`
	MetroName        string `json:"metroName"`
	DistrictName     string `json:"districtName"`
}

type robotaResponse struct {
	Total     int             `json:"total"`
	Documents []robotaVacancy `json:"documents"`
}

type RobotaAdapter struct{}

func (RobotaAdapter) Key() string          { return "robota" }
func (RobotaAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func (a RobotaAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	keywords := strings.TrimSpace(query.Keywords)
	if keywords == "" {
		return nil, fmt.Errorf("robota: keywords are required")
	}

	allJobs := make([]dto.NormalizedJob, 0)

	firstRes, err := a.fetchPage(ctx, keywords, 0)
	if err != nil {
		return nil, fmt.Errorf("robota: search: %w", err)
	}
	if firstRes == nil || len(firstRes.Documents) == 0 {
		slog.Warn("robota: returned 0 jobs", "keywords", keywords)
		return nil, nil
	}

	allJobs = append(allJobs, a.mapJobs(firstRes.Documents)...)

	totalPages := firstRes.Total / robotaVacanciesPerPage
	if firstRes.Total%robotaVacanciesPerPage > 0 {
		totalPages++
	}
	if totalPages > robotaMaxPages {
		totalPages = robotaMaxPages
	}

	for page := 1; page < totalPages; page++ {
		select {
		case <-ctx.Done():
			return allJobs, ctx.Err()
		default:
		}

		res, err := a.fetchPage(ctx, keywords, page)
		if err != nil {
			slog.Warn("robota: page fetch failed", "page", page, "error", err)
			break
		}
		if res == nil || len(res.Documents) == 0 {
			break
		}
		allJobs = append(allJobs, a.mapJobs(res.Documents)...)
	}

	return allJobs, nil
}

func (a RobotaAdapter) fetchPage(ctx context.Context, keywords string, page int) (*robotaResponse, error) {
	params := url.Values{}
	params.Set("keyWords", keywords)
	params.Set("count", strconv.Itoa(robotaVacanciesPerPage))
	params.Set("page", strconv.Itoa(page))

	var res robotaResponse
	if err := jobsources.GetJSON(ctx, nil, robotaAPIBase+"/vacancy/search", params, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (a RobotaAdapter) mapJobs(vacancies []robotaVacancy) []dto.NormalizedJob {
	jobs := make([]dto.NormalizedJob, 0, len(vacancies))
	for _, v := range vacancies {
		if v.Name == "" {
			continue
		}

		company := v.CompanyName
		if company == "" {
			company = "Unknown"
		}

		var salaryRaw *string
		if v.SalaryFrom > 0 || v.SalaryTo > 0 {
			var parts []string
			if v.SalaryFrom > 0 {
				parts = append(parts, formatSalary(v.SalaryFrom))
			}
			if v.SalaryTo > 0 {
				parts = append(parts, formatSalary(v.SalaryTo))
			}
			raw := strings.Join(parts, " - ")
			if v.SalaryComment != "" {
				raw += " (" + v.SalaryComment + ")"
			}
			salaryRaw = &raw
		}

		location := strings.TrimSpace(v.CityName)
		if v.MetroName != "" {
			location += ", " + v.MetroName
		}

		var postedAt *string
		if v.Date != "" {
			t, err := time.Parse(time.RFC3339Nano, v.Date)
			if err == nil {
				rfc3339 := t.Format(time.RFC3339)
				postedAt = &rfc3339
			}
		}

		desc := strings.TrimSpace(v.ShortDescription)

		jobURL := fmt.Sprintf("https://robota.ua/company/%d/vacancy/%d", v.NotebookID, v.ID)

		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "robota",
			ExternalID:  jobsources.Ptr(strconv.FormatUint(v.ID, 10)),
			Title:       v.Name,
			Company:     company,
			Location:    jobsources.NilIfEmpty(location),
			Remote:      false,
			SalaryRaw:   salaryRaw,
			URL:         jobURL,
			Description: desc,
			PostedAt:    postedAt,
			Raw:         v,
		})
	}
	return jobs
}

func formatSalary(v uint32) string {
	return strconv.FormatUint(uint64(v), 10) + " грн"
}

func (RobotaAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	var res robotaResponse
	if err := jobsources.GetJSON(ctx, nil, robotaAPIBase+"/vacancy/search", url.Values{"keyWords": {"developer"}, "count": {"1"}}, &res); err != nil {
		return false, nil
	}
	return len(res.Documents) > 0, nil
}
