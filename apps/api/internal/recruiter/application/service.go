package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/llm"

	"github.com/job-finder/api/internal/recruiter/domain"
)

// Repository is the outbound persistence port Resolve needs. *sqlcgen.Queries
// satisfies it structurally.
type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpsertJobContact(ctx context.Context, arg sqlcgen.UpsertJobContactParams) (sqlcgen.JobContact, error)
	ListJobContactsByJob(ctx context.Context, jobId pgtype.UUID) ([]sqlcgen.JobContact, error)
	// GetCompanyByNormalizedName backs the company-page source (US2): it
	// reads the Company.website plan 004 populates, keyed by the job's
	// normalized company name. Never written by this package.
	GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error)
}

// Service orchestrates the resolution sources and the JobContact
// persistence, serving the GET/refresh contact endpoints.
type Service struct {
	q            Repository
	llmc         llm.Provider
	postingModel string
	// scraping fetches the company-page source's target URL (US2). Reused
	// from the shared scraping.Service (the same fetch abstraction
	// companyintel's HeadcountScraper uses) rather than a forked client.
	scraping ScrapingService
	// linkedInEnabled gates the LinkedIn source (US2); read once at process
	// start from LINKEDIN_SCRAPE_ENABLED (FR-004, FR-019). When false the
	// LinkedIn source is never constructed/invoked.
	linkedInEnabled bool
}

// ScrapingService is the outbound fetch port the company-page and LinkedIn
// sources need. *scraping.Service satisfies it structurally.
type ScrapingService interface {
	FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error)
}

func NewService(q Repository, llmc llm.Provider, postingModel string, scraping ScrapingService, linkedInEnabled bool) *Service {
	return &Service{q: q, llmc: llmc, postingModel: postingModel, scraping: scraping, linkedInEnabled: linkedInEnabled}
}

// ListContacts returns every stored JobContact for a job, ordered
// best-first (FR-012, confidence desc / source-priority / name tie-break —
// see ListJobContactsByJob). Always a non-nil (possibly empty) slice, so
// callers/handlers can serialize "[]" rather than "null" for the no-contact
// state (FR-009).
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

// Resolve runs every enabled source for one job — currently posting-text
// only; the company-page and LinkedIn sources are added in US2 — and
// upserts whatever each source grounds. Each source fails independently
// (FR-015): a source error is logged and skipped, never propagated, so
// the sources that did succeed are still persisted. Zero contacts overall
// is success, not an error (FR-016). Returns the freshly-read contact set.
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

// resolutionSource is one producer of contacts, isolated so its failure
// never affects the others (FR-015).
type resolutionSource struct {
	name string
	run  func(ctx context.Context) ([]domain.ResolvedContact, error)
}

// sources builds the ordered list of resolution sources to run for one
// job. Posting-text always runs; the company-page source runs whenever the
// job's company has a website on file (plan 004); the LinkedIn source is
// only included at all when LINKEDIN_SCRAPE_ENABLED is true (FR-004) — when
// false it is never constructed, let alone invoked, so a disabled run makes
// zero LinkedIn requests (SC-004).
func (s *Service) sources(ctx context.Context, job sqlcgen.Job) []resolutionSource {
	sources := []resolutionSource{
		{
			name: domain.SourcePosting,
			run: func(ctx context.Context) ([]domain.ResolvedContact, error) {
				contact, err := ExtractPostingContact(ctx, s.llmc, s.postingModel, job.Description)
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

// toDtoSlice converts stored JobContact rows to the wire DTO, always
// returning a non-nil slice (an empty result renders as JSON "[]", not
// "null") so the dashboard's zero-contact state has a stable shape.
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
