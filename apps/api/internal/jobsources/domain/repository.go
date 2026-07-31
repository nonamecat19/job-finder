package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the jobsources use-case.
// Method names mirror the generated queries so *sqlcgen.Queries satisfies it
// structurally, with no hand-written adapter in between.
type Repository interface {
	GetJobSourceByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
	ListJobSources(ctx context.Context) ([]sqlcgen.JobSource, error)
	SetJobSourceConfig(ctx context.Context, arg sqlcgen.SetJobSourceConfigParams) error
	SetJobSourceEnabled(ctx context.Context, arg sqlcgen.SetJobSourceEnabledParams) error
	SetJobSourceHealthy(ctx context.Context, arg sqlcgen.SetJobSourceHealthyParams) error
	UpsertJobSource(ctx context.Context, arg sqlcgen.UpsertJobSourceParams) error
}

// BatchRepository is the outbound port for the batched ingest persist phase.
// It is declared separately from SearchRepository because the persist phase
// runs inside db.WithinTx, which hands back a transaction-bound
// *sqlcgen.Queries rather than the handler's own repository value — both
// satisfy this structurally, so the same code serves both paths.
//
// FinishSourceRunOk belongs here because the run's totals must commit with
// the postings they describe (FR-011).
type BatchRepository interface {
	GetJobsByDedupeKeys(ctx context.Context, dedupeKeys []string) ([]sqlcgen.GetJobsByDedupeKeysRow, error)
	FindJobsByCompanies(ctx context.Context, arg sqlcgen.FindJobsByCompaniesParams) ([]sqlcgen.FindJobsByCompaniesRow, error)
	BulkInsertJobs(ctx context.Context, arg sqlcgen.BulkInsertJobsParams) ([]sqlcgen.BulkInsertJobsRow, error)
	BulkRecordJobReposts(ctx context.Context, arg sqlcgen.BulkRecordJobRepostsParams) (int64, error)
	BulkMergeJobBoards(ctx context.Context, arg sqlcgen.BulkMergeJobBoardsParams) (int64, error)
	BulkInsertActivities(ctx context.Context, arg sqlcgen.BulkInsertActivitiesParams) ([]sqlcgen.BulkInsertActivitiesRow, error)
	FinishSourceRunOk(ctx context.Context, arg sqlcgen.FinishSourceRunOkParams) error
}

// TxRunner is the atomicity port for the ingest persist phase, mirroring
// applications/domain.TxRunner. *db.DB satisfies it structurally. Without one
// injected the handler degrades to non-transactional sequential writes, which
// is what the unit-test fakes use.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}
