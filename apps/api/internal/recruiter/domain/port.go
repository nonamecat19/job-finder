package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
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

// ScrapingService is the outbound fetch port the company-page and LinkedIn
// sources need. *scraping.Service satisfies it structurally.
type ScrapingService interface {
	FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error)
}
