package notifier_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/notifier"
)

var _ notifier.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	notifier.Repository
	jobs     map[string]sqlcgen.Job
	inserted []sqlcgen.FreshMatchNotification
	count    int64
	profiles []sqlcgen.Profile
}

func (f *fakeRepo) GetJobByID(_ context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return f.jobs[dbutil.UUIDString(id)], nil
}

func (f *fakeRepo) InsertFreshMatchNotification(_ context.Context, arg sqlcgen.InsertFreshMatchNotificationParams) (sqlcgen.FreshMatchNotification, error) {
	n := sqlcgen.FreshMatchNotification{
		ID:            newUUID(),
		JobId:         arg.JobId,
		MatchResultId: arg.MatchResultId,
		ProfileId:     arg.ProfileId,
		Fresh:         arg.Fresh,
		Seen:          false,
		CreatedAt:     dbutil.NowTimestamp(),
	}
	f.inserted = append(f.inserted, n)
	return n, nil
}

func (f *fakeRepo) CountRecentNotificationsByProfile(_ context.Context, _ sqlcgen.CountRecentNotificationsByProfileParams) (int64, error) {
	return f.count, nil
}

func (f *fakeRepo) ListProfiles(_ context.Context) ([]sqlcgen.Profile, error) {
	return f.profiles, nil
}

func newUUID() pgtype.UUID {
	id := uuid.New()
	return pgtype.UUID{Bytes: id, Valid: true}
}

func TestIsFresh(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		postedAt pgtype.Timestamp
		want     bool
	}{
		{
			name:     "just now",
			postedAt: pgtype.Timestamp{Time: now, Valid: true},
			want:     true,
		},
		{
			name:     "1 hour ago",
			postedAt: pgtype.Timestamp{Time: now.Add(-1 * time.Hour), Valid: true},
			want:     true,
		},
		{
			name:     "23 hours ago",
			postedAt: pgtype.Timestamp{Time: now.Add(-23 * time.Hour), Valid: true},
			want:     true,
		},
		{
			name:     "25 hours ago (stale)",
			postedAt: pgtype.Timestamp{Time: now.Add(-25 * time.Hour), Valid: true},
			want:     false,
		},
		{
			name:     "48 hours ago (stale)",
			postedAt: pgtype.Timestamp{Time: now.Add(-48 * time.Hour), Valid: true},
			want:     false,
		},
		{
			name:     "no postedAt",
			postedAt: pgtype.Timestamp{Valid: false},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notifier.IsFresh(tt.postedAt)
			if got != tt.want {
				t.Errorf("IsFresh(%v) = %v, want %v", tt.postedAt.Time, got, tt.want)
			}
		})
	}
}

func TestMaybeNotify_BelowThreshold(t *testing.T) {
	r := &fakeRepo{
		profiles: []sqlcgen.Profile{{ID: newUUID()}},
	}
	svc := notifier.NewService(r, notifier.WithMatchThreshold(70))

	ctx := context.Background()
	svc.MaybeNotify(ctx, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", 50)

	if len(r.inserted) != 0 {
		t.Errorf("expected 0 notifications for score below threshold, got %d", len(r.inserted))
	}
}

func TestMaybeNotify_AboveThreshold(t *testing.T) {
	jobID := newUUID()
	mrID := newUUID()
	jobIDStr := dbutil.UUIDString(jobID)
	mrIDStr := dbutil.UUIDString(mrID)
	r := &fakeRepo{
		profiles: []sqlcgen.Profile{{ID: newUUID()}},
		jobs: map[string]sqlcgen.Job{
			jobIDStr: {
				ID:       jobID,
				PostedAt: pgtype.Timestamp{Time: time.Now().UTC(), Valid: true},
			},
		},
	}
	svc := notifier.NewService(r, notifier.WithMatchThreshold(70))

	ctx := context.Background()
	svc.MaybeNotify(ctx, jobIDStr, mrIDStr, 85)

	if len(r.inserted) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(r.inserted))
	}
	if !r.inserted[0].Fresh {
		t.Error("expected notification to be fresh")
	}
}

func TestMaybeNotify_StaleJob(t *testing.T) {
	jobID := newUUID()
	mrID := newUUID()
	jobIDStr := dbutil.UUIDString(jobID)
	mrIDStr := dbutil.UUIDString(mrID)
	r := &fakeRepo{
		profiles: []sqlcgen.Profile{{ID: newUUID()}},
		jobs: map[string]sqlcgen.Job{
			jobIDStr: {
				ID:       jobID,
				PostedAt: pgtype.Timestamp{Time: time.Now().UTC().Add(-48 * time.Hour), Valid: true},
			},
		},
	}
	svc := notifier.NewService(r, notifier.WithMatchThreshold(70))

	ctx := context.Background()
	svc.MaybeNotify(ctx, jobIDStr, mrIDStr, 85)

	if len(r.inserted) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(r.inserted))
	}
	if r.inserted[0].Fresh {
		t.Error("expected notification to be stale (not fresh)")
	}
}

func TestMaybeNotify_RateLimitReached(t *testing.T) {
	jobID := newUUID()
	mrID := newUUID()
	jobIDStr := dbutil.UUIDString(jobID)
	mrIDStr := dbutil.UUIDString(mrID)
	r := &fakeRepo{
		profiles: []sqlcgen.Profile{{ID: newUUID()}},
		jobs: map[string]sqlcgen.Job{
			jobIDStr: {
				ID:       jobID,
				PostedAt: pgtype.Timestamp{Time: time.Now().UTC(), Valid: true},
			},
		},
		count: 10,
	}
	svc := notifier.NewService(r,
		notifier.WithMatchThreshold(70),
		notifier.WithRateLimitCap(10),
	)

	ctx := context.Background()
	svc.MaybeNotify(ctx, jobIDStr, mrIDStr, 85)

	if len(r.inserted) != 0 {
		t.Errorf("expected 0 notifications when rate limit reached, got %d", len(r.inserted))
	}
}

func TestMaybeNotify_UnderRateLimit(t *testing.T) {
	jobID := newUUID()
	mrID := newUUID()
	jobIDStr := dbutil.UUIDString(jobID)
	mrIDStr := dbutil.UUIDString(mrID)
	r := &fakeRepo{
		profiles: []sqlcgen.Profile{{ID: newUUID()}},
		jobs: map[string]sqlcgen.Job{
			jobIDStr: {
				ID:       jobID,
				PostedAt: pgtype.Timestamp{Time: time.Now().UTC(), Valid: true},
			},
		},
		count: 5,
	}
	svc := notifier.NewService(r,
		notifier.WithMatchThreshold(70),
		notifier.WithRateLimitCap(10),
	)

	ctx := context.Background()
	svc.MaybeNotify(ctx, jobIDStr, mrIDStr, 85)

	if len(r.inserted) != 1 {
		t.Fatalf("expected 1 notification when under rate limit, got %d", len(r.inserted))
	}
	if !r.inserted[0].Fresh {
		t.Error("expected notification to be fresh")
	}
}
