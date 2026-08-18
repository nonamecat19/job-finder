package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nonamecat19/job-scraper/ports"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	jobsources "github.com/job-finder/api/internal/jobsources/domain"
)

type Repository interface {
	jobsources.BatchRepository

	InsertSourceRun(ctx context.Context, arg sqlcgen.InsertSourceRunParams) (sqlcgen.SourceRun, error)
	FinishSourceRunError(ctx context.Context, arg sqlcgen.FinishSourceRunErrorParams) error
	GetJobByDedupeKey(ctx context.Context, dedupeKey string) (pgtype.UUID, error)
	TouchSubscriptionLastRun(ctx context.Context, id pgtype.UUID) error
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

type SourceProvider interface {
	GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
	DecryptConfig(stored []byte) map[string]any
}

type SubscriptionEnsurer interface {
	EnsureManualSubscription(ctx context.Context, sourceKey string) (sqlcgen.Subscription, error)
}

type AdapterRegistry interface {
	All() []ports.JobSource
}

type JobReader interface {
	Get(ctx context.Context, id string) (dto.JobDto, error)
}

type Enqueuer interface {
	EnqueueContext(ctx context.Context, workType string, payload []byte) error
}
