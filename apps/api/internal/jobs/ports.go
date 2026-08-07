package jobs

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	activity.Store

	CountJobs(ctx context.Context, arg sqlcgen.CountJobsParams) (int64, error)
	ListJobsByScore(ctx context.Context, arg sqlcgen.ListJobsByScoreParams) ([]sqlcgen.ListJobsByScoreRow, error)
	ListJobsByDate(ctx context.Context, arg sqlcgen.ListJobsByDateParams) ([]sqlcgen.ListJobsByDateRow, error)
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	GetMatchResultByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.MatchResult, error)
	GetJobDocuments(ctx context.Context, jobID pgtype.UUID) ([]sqlcgen.GeneratedDocument, error)
	GetApplicationByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.Application, error)
	GetJobSignal(ctx context.Context, arg sqlcgen.GetJobSignalParams) (sqlcgen.JobSignal, error)
	ListJobSignalsByJobIds(ctx context.Context, arg sqlcgen.ListJobSignalsByJobIdsParams) ([]sqlcgen.JobSignal, error)
	UpdateJobStatus(ctx context.Context, arg sqlcgen.UpdateJobStatusParams) (sqlcgen.Job, error)
	UpsertApplicationStatus(ctx context.Context, arg sqlcgen.UpsertApplicationStatusParams) error
	GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error)
	ProfileHasConfig(ctx context.Context, id pgtype.UUID) (interface{}, error)
	DeleteAllJobs(ctx context.Context) (int64, error)
}

type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
