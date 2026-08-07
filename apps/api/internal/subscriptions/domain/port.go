package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	CreateSubscription(ctx context.Context, arg sqlcgen.CreateSubscriptionParams) (sqlcgen.Subscription, error)
	DeleteSubscription(ctx context.Context, id pgtype.UUID) error
	ListSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error)
	ListSubscriptionsBySource(ctx context.Context, sourceKey string) ([]sqlcgen.Subscription, error)
	UpdateSubscription(ctx context.Context, arg sqlcgen.UpdateSubscriptionParams) (sqlcgen.Subscription, error)
}

type SourceEnsurer interface {
	GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
}
