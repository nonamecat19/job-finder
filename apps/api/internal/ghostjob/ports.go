package ghostjob

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port the ghost-job use-case depends
// on. *sqlcgen.Queries satisfies it structurally, mirroring
// matching.Repository / salary.Repository's hexagonal seam.
type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	CountRepostsByDedupeKey(ctx context.Context, dedupekey string) (int32, error)
	ListJobsForCrossBoardCheck(ctx context.Context, id pgtype.UUID) ([]sqlcgen.ListJobsForCrossBoardCheckRow, error)
	CountAlwaysHiringByCompany(ctx context.Context, lower string) (int32, error)
	UpsertJobSignal(ctx context.Context, arg sqlcgen.UpsertJobSignalParams) (sqlcgen.JobSignal, error)
	GetJobSignal(ctx context.Context, arg sqlcgen.GetJobSignalParams) (sqlcgen.JobSignal, error)
}
