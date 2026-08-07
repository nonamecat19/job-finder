package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetJobSourceByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
	ListJobSources(ctx context.Context) ([]sqlcgen.JobSource, error)
	SetJobSourceConfig(ctx context.Context, arg sqlcgen.SetJobSourceConfigParams) error
	SetJobSourceEnabled(ctx context.Context, arg sqlcgen.SetJobSourceEnabledParams) error
	SetJobSourceHealthy(ctx context.Context, arg sqlcgen.SetJobSourceHealthyParams) error
	UpsertJobSource(ctx context.Context, arg sqlcgen.UpsertJobSourceParams) error
}

type BatchRepository interface {
	GetJobsByDedupeKeys(ctx context.Context, dedupeKeys []string) ([]sqlcgen.GetJobsByDedupeKeysRow, error)
	FindJobsByCompanies(ctx context.Context, arg sqlcgen.FindJobsByCompaniesParams) ([]sqlcgen.FindJobsByCompaniesRow, error)
	BulkInsertJobs(ctx context.Context, arg sqlcgen.BulkInsertJobsParams) ([]sqlcgen.BulkInsertJobsRow, error)
	BulkRecordJobReposts(ctx context.Context, arg sqlcgen.BulkRecordJobRepostsParams) (int64, error)
	BulkMergeJobBoards(ctx context.Context, arg sqlcgen.BulkMergeJobBoardsParams) (int64, error)
	BulkInsertActivities(ctx context.Context, arg sqlcgen.BulkInsertActivitiesParams) ([]sqlcgen.BulkInsertActivitiesRow, error)
	FinishSourceRunOk(ctx context.Context, arg sqlcgen.FinishSourceRunOkParams) error
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}
