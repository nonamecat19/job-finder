package enrichment

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	activity.Store

	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	ListJobsNeedingDetail(ctx context.Context, arg sqlcgen.ListJobsNeedingDetailParams) ([]sqlcgen.Job, error)
	UpdateJobDetail(ctx context.Context, arg sqlcgen.UpdateJobDetailParams) (sqlcgen.Job, error)
	ClearJobDetailScrapedAt(ctx context.Context, id pgtype.UUID) error
}

type Enqueuer interface {
	EnqueueContext(ctx context.Context, workType string, payload []byte) error
}
