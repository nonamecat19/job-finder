// Package domain holds the application-tracking bounded context's core
// model: the Repository persistence port, the TxRunner atomicity port, and
// the ErrNotFound sentinel.
package domain

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// ErrNotFound is a sentinel so callers (the HTTP layer) can distinguish
// "no such application" (404) from other Update failures like an invalid
// status value (400) — mirrors NestJS's NotFoundException vs
// BadRequestException split in applications.service.ts:25,28.
var ErrNotFound = errors.New("application not found")

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
	InsertApplicationOutcome(ctx context.Context, arg sqlcgen.InsertApplicationOutcomeParams) (sqlcgen.ApplicationOutcome, error)
	ListApplicationOutcomes(ctx context.Context, applicationID pgtype.UUID) ([]sqlcgen.ApplicationOutcome, error)
}

// TxRunner is the optional atomicity port. *db.DB satisfies it structurally.
// When one is injected, a status change and the "ApplicationOutcome" event it
// records commit together or not at all (spec 010); without one the service
// degrades to sequential writes, which is what the unit-test fakes use.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}
