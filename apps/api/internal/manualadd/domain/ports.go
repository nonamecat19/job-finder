package domain

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	jsadapter "github.com/nonamecat19/jobscraper/adapter"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	jobsources "github.com/job-finder/api/internal/jobsources/domain"
)

// Repository is everything manual add needs from the database beyond what the
// shared ingest path already takes. It embeds the ingest batch port so one
// value can serve both, which is what makes a manual add byte-for-byte what a
// crawl produces.
type Repository interface {
	jobsources.BatchRepository

	InsertSourceRun(ctx context.Context, arg sqlcgen.InsertSourceRunParams) (sqlcgen.SourceRun, error)
	FinishSourceRunError(ctx context.Context, arg sqlcgen.FinishSourceRunErrorParams) error
	GetJobByDedupeKey(ctx context.Context, dedupeKey string) (pgtype.UUID, error)
	TouchSubscriptionLastRun(ctx context.Context, id pgtype.UUID) error
}

// TxRunner makes the whole persist atomic, so a failure or a timeout mid-write
// leaves no vacancy behind (FR-021).
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error
}

// SourceProvider resolves a source key to its row and its decrypted config.
type SourceProvider interface {
	GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
	DecryptConfig(stored []byte) map[string]any
}

// SubscriptionEnsurer owns the one manual subscription per source. Manual rows
// are created here and nowhere else (FR-015).
type SubscriptionEnsurer interface {
	EnsureManualSubscription(ctx context.Context, sourceKey string) (sqlcgen.Subscription, error)
}

// AdapterRegistry is the ordered set of adapters URL resolution walks. Order is
// deterministic and the first PostingReader to claim a URL wins (D2).
type AdapterRegistry interface {
	All() []jsadapter.Adapter
}

// JobReader turns a freshly written row into the same JobDto the feed and
// detail views already consume — manual add returns no bespoke shape (FR-009).
type JobReader interface {
	Get(ctx context.Context, id string) (dto.JobDto, error)
}

// Enqueuer schedules the post-ingest work. Those enqueues happen after the
// vacancy is committed and are deliberately outside the 30 s budget (FR-003d).
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
