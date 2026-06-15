package ingestion

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
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
	q       *sqlcgen.Queries
	service *Service
}

func NewScheduler(q *sqlcgen.Queries, service *Service) *Scheduler {
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
		slog.Info("search due", "search", search.Name, "cron", search.Cron)
		if _, err := s.service.RunSearch(ctx, dbutil.UUIDString(search.ID)); err != nil {
			slog.Error("scheduler: run search failed", "search", search.Name, "error", err)
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
