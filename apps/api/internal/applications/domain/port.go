package domain

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

var ErrNotFound = errors.New("application not found")

type Repository interface {
	GetApplicationByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Application, error)
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	ListApplications(ctx context.Context, status *string) ([]sqlcgen.ListApplicationsRow, error)
	RecentRunsJoined(ctx context.Context, limit int32) ([]sqlcgen.RecentRunsJoinedRow, error)
	StatsHighFit(ctx context.Context) (int64, error)
	StatsJobsLast24h(ctx context.Context, ingestedAt pgtype.Timestamp) (int64, error)
	StatsJobsTotal(ctx context.Context) (int64, error)
	StatsPipeline(ctx context.Context) ([]sqlcgen.StatsPipelineRow, error)
	UpdateApplication(ctx context.Context, arg sqlcgen.UpdateApplicationParams) (sqlcgen.Application, error)
	UpdateJobStatus(ctx context.Context, arg sqlcgen.UpdateJobStatusParams) (sqlcgen.Job, error)
	InsertApplicationOutcome(ctx context.Context, arg sqlcgen.InsertApplicationOutcomeParams) (sqlcgen.ApplicationOutcome, error)
	ListApplicationOutcomes(ctx context.Context, applicationID pgtype.UUID) ([]sqlcgen.ApplicationOutcome, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}
