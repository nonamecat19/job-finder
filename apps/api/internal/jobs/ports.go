package jobs

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port the jobs use-case depends on.
// *sqlcgen.Queries satisfies it structurally, so the concrete sqlc adapter is
// injected at wire-up while the service is testable against a fake. This is the
// hexagonal seam: the use-case owns the interface, the infrastructure conforms.
type Repository interface {
	// EnqueueGeneration records an activity run, so the port also serves the
	// activity.Store methods (one injected value covers both).
	activity.Store

	CountJobs(ctx context.Context, arg sqlcgen.CountJobsParams) (int64, error)
	ListJobsByScore(ctx context.Context, arg sqlcgen.ListJobsByScoreParams) ([]sqlcgen.ListJobsByScoreRow, error)
	ListJobsByDate(ctx context.Context, arg sqlcgen.ListJobsByDateParams) ([]sqlcgen.ListJobsByDateRow, error)
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	GetMatchResultByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.MatchResult, error)
	GetJobDocuments(ctx context.Context, jobID pgtype.UUID) ([]sqlcgen.GeneratedDocument, error)
	GetApplicationByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.Application, error)
	UpdateJobStatus(ctx context.Context, arg sqlcgen.UpdateJobStatusParams) (sqlcgen.Job, error)
	UpsertApplicationStatus(ctx context.Context, arg sqlcgen.UpsertApplicationStatusParams) error
	GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error)
	ProfileHasConfig(ctx context.Context, id pgtype.UUID) (interface{}, error)
	DeleteAllJobs(ctx context.Context) (int64, error)
}

// Enqueuer is the outbound async-task port. *asynq.Client satisfies it.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
