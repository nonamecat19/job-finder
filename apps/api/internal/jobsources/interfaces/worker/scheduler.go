package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
)

// Scheduler replicates ingestion.scheduler.ts: every 5 minutes, for each
// enabled SavedSearch, run it if its cron slot has passed since lastRunAt.
//
// due = !lastRunAt || lastRunAt < prev(cron slot before now)
//
// We compute this as due = !lastRunAt || Next(lastRunAt) <= now, which is
// mathematically equivalent (Next() is the only primitive robfig/cron
// exposes) — see the derivation in internal/ingestion doc comments.
type Scheduler struct {
	q       domain.SearchRepository
	service *application.SearchService
}

func NewScheduler(q domain.SearchRepository, service *application.SearchService) *Scheduler {
	return &Scheduler{q: q, service: service}
}

// Tick runs one due-check pass. Call this every 5 minutes from main.
func (s *Scheduler) Tick(ctx context.Context) {
	searches, err := s.q.ListEnabledSavedSearches(ctx)
	if err != nil {
		slog.Error("scheduler: list enabled searches failed", "error", err)
		return
	}
	now := time.Now()
	for _, search := range searches {
		schedule, err := cron.ParseStandard(search.Cron)
		if err != nil {
			slog.Error("scheduler: bad cron expression", "search", search.Name, "cron", search.Cron, "error", err)
			continue
		}
		due := !search.LastRunAt.Valid || !schedule.Next(search.LastRunAt.Time).After(now)
		if !due {
			continue
		}
		// Claim the slot before enqueueing anything. The CAS fails for a
		// second scheduler racing on the same search (and for this one if the
		// search ran between the list and now), so a due search is scraped
		// once per slot no matter how many API replicas are running.
		if _, err := s.q.ClaimSavedSearchRun(ctx, sqlcgen.ClaimSavedSearchRunParams{
			ID:                search.ID,
			ExpectedLastRunAt: search.LastRunAt,
		}); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				slog.Error("scheduler: claim search failed", "search", search.Name, "error", err)
			}
			continue
		}
		slog.Info("search due", "search", search.Name, "cron", search.Cron)
		if _, err := s.service.RunSearch(ctx, dbutil.UUIDString(search.ID)); err != nil {
			slog.Error("scheduler: run search failed", "search", search.Name, "error", err)
		}
	}

	s.tickSubscriptions(ctx, now)

	if _, err := s.service.ReconcileUnmatched(ctx); err != nil {
		slog.Error("scheduler: reconcile unmatched jobs failed", "error", err)
	}

	cutoff := pgtype.Timestamp{Time: now.AddDate(0, 0, -7), Valid: true}
	if err := s.q.DeleteActivityRunsBefore(ctx, cutoff); err != nil {
		slog.Error("scheduler: activity retention sweep failed", "error", err)
	}
}

// tickSubscriptions runs the due-and-claim pass over enabled subscriptions.
// Subscriptions have carried a "lastRunAt" since they were introduced, but
// nothing ever set it on a schedule — only a manual Run did — so a
// subscription silently stayed as stale as the last time someone pressed the
// button. Same rules as saved searches: parse its cron, compare against
// lastRunAt, claim the slot by CAS, then enqueue.
func (s *Scheduler) tickSubscriptions(ctx context.Context, now time.Time) {
	subs, err := s.q.ListEnabledSubscriptions(ctx)
	if err != nil {
		slog.Error("scheduler: list enabled subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		schedule, err := cron.ParseStandard(sub.Cron)
		if err != nil {
			slog.Error("scheduler: bad subscription cron expression", "subscription", sub.Url, "cron", sub.Cron, "error", err)
			continue
		}
		if sub.LastRunAt.Valid && schedule.Next(sub.LastRunAt.Time).After(now) {
			continue
		}
		if _, err := s.q.ClaimSubscriptionRun(ctx, sqlcgen.ClaimSubscriptionRunParams{
			ID:                sub.ID,
			ExpectedLastRunAt: sub.LastRunAt,
		}); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				slog.Error("scheduler: claim subscription failed", "subscription", sub.Url, "error", err)
			}
			continue
		}
		slog.Info("subscription due", "subscription", sub.Url, "source", sub.SourceKey, "cron", sub.Cron)
		if err := s.service.RunSubscription(ctx, dbutil.UUIDString(sub.ID)); err != nil {
			slog.Error("scheduler: run subscription failed", "subscription", sub.Url, "error", err)
		}
	}
}

// Run starts a ticker that calls Tick every 5 minutes until ctx is done,
// mirroring @Cron('*/5 * * * *').
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}
