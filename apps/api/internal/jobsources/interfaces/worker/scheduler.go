package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/platform/langfuseretention"
	"github.com/job-finder/api/internal/platform/observability"
)

type Scheduler struct {
	q       domain.SearchRepository
	service *application.SearchService

	pruner Pruner

	payloadPruner PayloadPruner
}

type Pruner interface {
	Prune(ctx context.Context) (observability.Report, error)
}

type PayloadPruner interface {
	Prune(ctx context.Context) (langfuseretention.Report, error)
}

func NewScheduler(q domain.SearchRepository, service *application.SearchService, pruner Pruner, payloadPruner PayloadPruner) *Scheduler {
	return &Scheduler{q: q, service: service, pruner: pruner, payloadPruner: payloadPruner}
}

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

	cutoff := dbutil.TimestampAt(now.AddDate(0, 0, -7))
	if err := s.q.DeleteActivityRunsBefore(ctx, cutoff); err != nil {
		slog.Error("scheduler: activity retention sweep failed", "error", err)
	}

	s.tickObservabilityRetention(ctx)
	s.tickPayloadRetention(ctx)
}

func (s *Scheduler) tickObservabilityRetention(ctx context.Context) {
	if s.pruner == nil {
		return
	}
	report, err := s.pruner.Prune(ctx)
	if err != nil {
		slog.Error("scheduler: observability retention sweep failed",
			"error", err, "deletedBeforeFailure", report.Deleted)
		return
	}
	if report.Skipped || report.Deleted == 0 {
		return
	}
	slog.Info("scheduler: observability retention sweep",
		"deleted", report.Deleted,
		"cutoff", report.Cutoff.Format(time.RFC3339),
		"truncated", report.Truncated)
}

func (s *Scheduler) tickPayloadRetention(ctx context.Context) {
	if s.payloadPruner == nil {
		return
	}
	report, err := s.payloadPruner.Prune(ctx)
	if err != nil {
		slog.Error("scheduler: langfuse payload retention sweep failed", "error", err)
		return
	}
	if report.Skipped || len(report.TablesPurged) == 0 {
		return
	}
	slog.Info("scheduler: langfuse payload retention sweep",
		"tables", report.TablesPurged,
		"cutoff", report.Cutoff.Format(time.RFC3339))
}

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
