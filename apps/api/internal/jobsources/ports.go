package jobsources

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the jobsources use-case.
// *sqlcgen.Queries satisfies it structurally.
type Repository interface {
	GetJobSourceByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
	ListJobSources(ctx context.Context) ([]sqlcgen.JobSource, error)
	SetJobSourceConfig(ctx context.Context, arg sqlcgen.SetJobSourceConfigParams) error
	SetJobSourceEnabled(ctx context.Context, arg sqlcgen.SetJobSourceEnabledParams) error
	SetJobSourceHealthy(ctx context.Context, arg sqlcgen.SetJobSourceHealthyParams) error
	UpsertJobSource(ctx context.Context, arg sqlcgen.UpsertJobSourceParams) error
}
