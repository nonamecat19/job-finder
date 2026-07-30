package enrichment

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the enrichment use-case.
// *sqlcgen.Queries satisfies it structurally. It embeds activity.Store because
// the handler starts/resumes an activity record from the same value
// (activity.New, activity.FromID).
type Repository interface {
	activity.Store

	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	ListJobsNeedingDetail(ctx context.Context, arg sqlcgen.ListJobsNeedingDetailParams) ([]sqlcgen.Job, error)
	UpdateJobDetail(ctx context.Context, arg sqlcgen.UpdateJobDetailParams) (sqlcgen.Job, error)
	ClearJobDetailScrapedAt(ctx context.Context, id pgtype.UUID) error
}

// Enqueuer is the outbound task-queue port. *asynq.Client satisfies it.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
