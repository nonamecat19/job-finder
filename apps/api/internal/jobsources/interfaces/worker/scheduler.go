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
	// pruner enforces the LLM observability retention window (036 FR-008). It
	// rides on this tick rather than getting a scheduler of its own because
	// this is already the platform's periodic-maintenance loop — the activity
	// retention sweep below is its sibling. Nil disables it.
	pruner Pruner
	// payloadPruner enforces the Langfuse trace payload retention window (047
	// FR-018/FR-018a, K5). Distinct from pruner above: that one deletes whole
	// trace rows through Langfuse's public API; this one blanks only the
	// input/output payload columns directly in ClickHouse, keeping step
	// sequence, timings, token counts and cost (FR-018). Nil disables it.
	payloadPruner PayloadPruner
}

// Pruner is the retention job this scheduler drives. It is an interface rather
// than the concrete type so the scheduler keeps no dependency on the collector,
// and so a deployment with no collector configured passes nil.
type Pruner interface {
	Prune(ctx context.Context) (observability.Report, error)
}

// PayloadPruner is the Langfuse trace payload retention job (047 K5). See
// internal/platform/langfuseretention for why it is a separate job from
// Pruner above.
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

// tickObservabilityRetention enforces the collector's retention window.
//
// The collector stores prompt bodies — which for this platform means the user's
// employment history, in plain text, in a service that will not expire it on
// its own. What each run deleted is logged, and a failure is logged as a
// failure (036 FR-008a): a retention guarantee nobody can check is how the
// window quietly stops being enforced while the documentation still claims it.
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

// tickPayloadRetention enforces FR-018/FR-018a: Langfuse trace step inputs
// and outputs — profile and resume content — older than
// LANGFUSE_PAYLOAD_RETENTION_DAYS are blanked, while step sequence, timings,
// token counts and cost stay in place.
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
