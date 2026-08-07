package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpsertJobContact(ctx context.Context, arg sqlcgen.UpsertJobContactParams) (sqlcgen.JobContact, error)
	ListJobContactsByJob(ctx context.Context, jobId pgtype.UUID) ([]sqlcgen.JobContact, error)
	GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error)
}

type ScrapingService interface {
	FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error)
}
