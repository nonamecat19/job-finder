package notifier

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// NotificationService implements httpapi.NotificationProvider for the
// GET /api/notifications and related endpoints.
type NotificationService struct {
	q    *sqlcgen.Queries
	prof ProfileResolver
}

type ProfileResolver interface {
	ListProfiles(ctx context.Context) ([]sqlcgen.Profile, error)
}

func NewNotificationService(q *sqlcgen.Queries, prof ProfileResolver) *NotificationService {
	return &NotificationService{q: q, prof: prof}
}

func (s *NotificationService) ListNotifications(ctx context.Context) ([]dto.FreshMatchNotificationDto, error) {
	profileUID, err := s.resolveProfileID(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.q.ListRecentNotificationsWithJob(ctx, sqlcgen.ListRecentNotificationsWithJobParams{
		ProfileId: profileUID,
		Limit:     50,
	})
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}

	out := make([]dto.FreshMatchNotificationDto, 0, len(rows))
	for _, r := range rows {
		d := dto.FreshMatchNotificationDto{
			ID:            dbutil.UUIDString(r.ID),
			JobId:         dbutil.UUIDString(r.JobId),
			MatchResultId: dbutil.UUIDString(r.MatchResultId),
			Fresh:         r.Fresh,
			Seen:          r.Seen,
			CreatedAt:     r.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			JobTitle:      &r.JobTitle,
			Company:       &r.Company,
			MatchScore:    r.MatchScore,
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *NotificationService) MarkSeen(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("invalid notification id: %w", err)
	}
	return s.q.MarkNotificationSeen(ctx, uid)
}

func (s *NotificationService) UnseenCount(ctx context.Context) (int64, error) {
	profileUID, err := s.resolveProfileID(ctx)
	if err != nil {
		return 0, err
	}
	return s.q.CountUnseenNotificationsByProfile(ctx, profileUID)
}

func (s *NotificationService) resolveProfileID(ctx context.Context) (pgtype.UUID, error) {
	rows, err := s.prof.ListProfiles(ctx)
	if err != nil || len(rows) == 0 {
		return pgtype.UUID{}, fmt.Errorf("no profile found: %w", err)
	}
	return rows[0].ID, nil
}
