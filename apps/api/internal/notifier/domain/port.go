package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port the write-path (MaybeNotify)
// needs. *sqlcgen.Queries satisfies it structurally.
type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	InsertFreshMatchNotification(ctx context.Context, arg sqlcgen.InsertFreshMatchNotificationParams) (sqlcgen.FreshMatchNotification, error)
	CountRecentNotificationsByProfile(ctx context.Context, arg sqlcgen.CountRecentNotificationsByProfileParams) (int64, error)
	ListProfiles(ctx context.Context) ([]sqlcgen.Profile, error)
}

// NotificationReader is the outbound persistence port the read-path
// (NotificationService) needs. *sqlcgen.Queries satisfies it structurally.
type NotificationReader interface {
	ListRecentNotificationsWithJob(ctx context.Context, arg sqlcgen.ListRecentNotificationsWithJobParams) ([]sqlcgen.ListRecentNotificationsWithJobRow, error)
	MarkNotificationSeen(ctx context.Context, id pgtype.UUID) error
	CountUnseenNotificationsByProfile(ctx context.Context, profileId pgtype.UUID) (int64, error)
}

// ProfileResolver is the cross-context port onto the user's profile,
// needed to scope both the write and read paths to the single active
// profile.
type ProfileResolver interface {
	ListProfiles(ctx context.Context) ([]sqlcgen.Profile, error)
}
