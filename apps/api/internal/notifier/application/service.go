package application

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/notifier/domain"
)

const (
	defaultMatchThreshold = 70
	defaultRateLimitCap   = 10
	defaultRateLimitHours = 1
)

type Service struct {
	q              domain.Repository
	matchThreshold int
	rateLimitCap   int
	rateLimitHours int
}

type Option func(*Service)

func WithMatchThreshold(t int) Option {
	return func(s *Service) { s.matchThreshold = t }
}

func WithRateLimitCap(c int) Option {
	return func(s *Service) { s.rateLimitCap = c }
}

func NewService(q domain.Repository, opts ...Option) *Service {
	s := &Service{
		q:              q,
		matchThreshold: defaultMatchThreshold,
		rateLimitCap:   defaultRateLimitCap,
		rateLimitHours: defaultRateLimitHours,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Service) MaybeNotify(ctx context.Context, jobID, matchResultID string, score int) {
	if score < s.matchThreshold {
		return
	}

	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		slog.Warn("notifier: invalid jobID", "jobID", jobID, "error", err)
		return
	}

	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		slog.Warn("notifier: job not found", "jobID", jobID, "error", err)
		return
	}

	profileUID, err := s.resolveProfileID(ctx)
	if err != nil {
		slog.Warn("notifier: no profile for rate limiting", "error", err)
		return
	}

	fresh := domain.IsFresh(job.PostedAt)

	count, err := s.q.CountRecentNotificationsByProfile(ctx, sqlcgen.CountRecentNotificationsByProfileParams{
		ProfileId: profileUID,
		Column2:   int32(s.rateLimitHours),
	})
	if err != nil {
		slog.Warn("notifier: rate limit check failed", "error", err)
		return
	}
	if count >= int64(s.rateLimitCap) {
		slog.Info("notifier: rate limit reached",
			"jobID", jobID,
			"profileID", dbutil.UUIDString(profileUID),
			"count", count,
			"cap", s.rateLimitCap,
			"windowHours", s.rateLimitHours,
		)
		return
	}

	mrUID, err := dbutil.ParseUUID(matchResultID)
	if err != nil {
		slog.Warn("notifier: invalid matchResultID", "matchResultID", matchResultID, "error", err)
		return
	}

	_, err = s.q.InsertFreshMatchNotification(ctx, sqlcgen.InsertFreshMatchNotificationParams{
		JobId:         uid,
		MatchResultId: mrUID,
		ProfileId:     profileUID,
		Fresh:         fresh,
	})
	if err != nil {
		slog.Warn("notifier: insert failed", "error", err)
		return
	}

	slog.Info("notifier: notification created",
		"jobID", jobID,
		"fresh", fresh,
		"score", score,
	)
}

func (s *Service) resolveProfileID(ctx context.Context) (pgtype.UUID, error) {
	rows, err := s.q.ListProfiles(ctx)
	if err != nil || len(rows) == 0 {
		return pgtype.UUID{}, fmt.Errorf("no profile found: %w", err)
	}
	return rows[0].ID, nil
}
