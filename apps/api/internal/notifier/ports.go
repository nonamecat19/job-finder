package notifier

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	InsertFreshMatchNotification(ctx context.Context, arg sqlcgen.InsertFreshMatchNotificationParams) (sqlcgen.FreshMatchNotification, error)
	CountRecentNotificationsByProfile(ctx context.Context, arg sqlcgen.CountRecentNotificationsByProfileParams) (int64, error)
	ListProfiles(ctx context.Context) ([]sqlcgen.Profile, error)
}
