package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"

	"github.com/job-finder/api/internal/recruiter/domain"
)

type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpsertJobContact(ctx context.Context, arg sqlcgen.UpsertJobContactParams) (sqlcgen.JobContact, error)
	ListJobContactsByJob(ctx context.Context, jobId pgtype.UUID) ([]sqlcgen.JobContact, error)
	GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error)
}

type Service struct {
	q               Repository
	scraping        ScrapingService
	linkedInEnabled bool
	extractor       ContactExtractor
}

type ScrapingService interface {
	FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error)
}

func NewService(q Repository, extractor ContactExtractor, scraping ScrapingService, linkedInEnabled bool) *Service {
	return &Service{q: q, scraping: scraping, linkedInEnabled: linkedInEnabled, extractor: extractor}
}

func (s *Service) ListContacts(ctx context.Context, jobID string) ([]dto.JobContactDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListJobContactsByJob(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("recruiter: list contacts for job %s: %w", jobID, err)
	}
	return toDtoSlice(rows), nil
}

func (s *Service) Resolve(ctx context.Context, jobID string) ([]dto.JobContactDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("recruiter: job %s not found: %w", jobID, err)
	}

	for _, src := range s.sources(ctx, job) {
		contacts, err := src.run(ctx)
		if err != nil {
			slog.Warn("recruiter: source failed", "source", src.name, "job", jobID, "error", err)
			continue
		}
		for _, c := range contacts {
			if _, err := s.upsert(ctx, uid, c); err != nil {
				slog.Warn("recruiter: persist contact failed", "source", src.name, "job", jobID, "error", err)
			}
		}
	}

	rows, err := s.q.ListJobContactsByJob(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("recruiter: list contacts for job %s: %w", jobID, err)
	}
	return toDtoSlice(rows), nil
}

type resolutionSource struct {
	name string
	run  func(ctx context.Context) ([]domain.ResolvedContact, error)
}

func (s *Service) sources(ctx context.Context, job sqlcgen.Job) []resolutionSource {
	sources := []resolutionSource{
		{
			name: domain.SourcePosting,
			run: func(ctx context.Context) ([]domain.ResolvedContact, error) {
				contact, err := s.extractPostingContact(ctx, job.Description)
				if err != nil {
					return nil, err
				}
				if contact == nil {
					return nil, nil
				}
				return []domain.ResolvedContact{*contact}, nil
			},
		},
		s.companyPageSource(job),
	}
	if s.linkedInEnabled {
		sources = append(sources, s.linkedInSource(job))
	}
	return sources
}

func (s *Service) upsert(ctx context.Context, jobID pgtype.UUID, c domain.ResolvedContact) (sqlcgen.JobContact, error) {
	return s.q.UpsertJobContact(ctx, sqlcgen.UpsertJobContactParams{
		JobId:       jobID,
		Name:        c.Name,
		Title:       c.Title,
		LinkedInUrl: c.LinkedInURL,
		Email:       c.Email,
		Phone:       c.Phone,
		Source:      c.Source,
		Confidence:  c.Confidence,
	})
}

func toDtoSlice(rows []sqlcgen.JobContact) []dto.JobContactDto {
	out := make([]dto.JobContactDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.JobContactDto{
			ID:          dbutil.UUIDString(r.ID),
			Name:        r.Name,
			Title:       r.Title,
			LinkedInUrl: r.LinkedInUrl,
			Email:       r.Email,
			Phone:       r.Phone,
			Source:      r.Source,
			Confidence:  r.Confidence,
			FetchedAt:   dbutil.Timestamp(r.FetchedAt),
		})
	}
	return out
}
