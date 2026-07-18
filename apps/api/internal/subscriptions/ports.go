package subscriptions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the subscriptions use-case.
// *sqlcgen.Queries satisfies it structurally.
type Repository interface {
	CreateSubscription(ctx context.Context, arg sqlcgen.CreateSubscriptionParams) (sqlcgen.Subscription, error)
	DeleteSubscription(ctx context.Context, id pgtype.UUID) error
	ListSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error)
	ListSubscriptionsBySource(ctx context.Context, sourceKey string) ([]sqlcgen.Subscription, error)
	UpdateSubscription(ctx context.Context, arg sqlcgen.UpdateSubscriptionParams) (sqlcgen.Subscription, error)
}

// SourceEnsurer validates a source key against the code-defined adapter
// registry and lazily materializes its JobSource row (needed to satisfy the
// Subscription -> JobSource FK). Source identity is hardcoded in the registry,
// not seeded in the db, so this is the single point that turns a key into a row.
type SourceEnsurer interface {
	GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
}
