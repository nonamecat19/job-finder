package applications

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the applications use-case.
// *sqlcgen.Queries satisfies it structurally, so the concrete sqlc adapter is
// injected at wire-up while the service stays testable against a fake.
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
}
