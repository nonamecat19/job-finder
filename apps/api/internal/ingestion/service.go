// Package ingestion ports modules/ingestion/*: SavedSearch CRUD, RunSearch
// (enqueue one ingest task per search × enabled source), the asynq ingest
// task handler (adapter.Search -> dedupe -> persist -> enqueue match), and
// the due-since-lastRunAt scheduler.
package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/queue"
)

type Service struct {
	q        Repository
	registry *jobsources.Registry
	sources  *jobsources.Service
	client   Enqueuer
}

func NewService(q Repository, registry *jobsources.Registry, sources *jobsources.Service, client Enqueuer) *Service {
	return &Service{q: q, registry: registry, sources: sources, client: client}
}

// ---------------------------------------------------------------------------
// SavedSearch CRUD (searches.controller.ts)
// ---------------------------------------------------------------------------

func (s *Service) ListSearches(ctx context.Context) ([]dto.SavedSearchDto, error) {
	rows, err := s.q.ListSavedSearches(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SavedSearchDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, savedSearchDto(r))
	}
	return out, nil
}

func (s *Service) CreateSearch(ctx context.Context, name string, query dto.SearchQuery, cron string, enabled bool) (*dto.SavedSearchDto, error) {
	if name == "" || query.Keywords == "" {
		return nil, fmt.Errorf("name and query.keywords are required")
	}
	q, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	if cron == "" {
		cron = "0 */6 * * *"
	}
	row, err := s.q.CreateSavedSearch(ctx, sqlcgen.CreateSavedSearchParams{
		Name:    name,
		Query:   q,
		Cron:    cron,
		Enabled: enabled,
	})
	if err != nil {
		return nil, err
	}
	dtoRow := savedSearchDto(row)
	return &dtoRow, nil
}

type UpdateSearchInput struct {
	Name    *string
	Query   *dto.SearchQuery
	Cron    *string
	Enabled *bool
}

func (s *Service) UpdateSearch(ctx context.Context, id string, in UpdateSearchInput) (*dto.SavedSearchDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return nil, err
	}
	params := sqlcgen.UpdateSavedSearchParams{ID: uid}
	if in.Name != nil {
		params.Name = in.Name
	}
	if in.Query != nil {
		q, err := json.Marshal(in.Query)
		if err != nil {
			return nil, err
		}
		params.Query = q
	}
	if in.Cron != nil {
		params.Cron = in.Cron
	}
	if in.Enabled != nil {
		params.Enabled = in.Enabled
	}
	row, err := s.q.UpdateSavedSearch(ctx, params)
	if err != nil {
		return nil, err
	}
	dtoRow := savedSearchDto(row)
	return &dtoRow, nil
}

func (s *Service) DeleteSearch(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteSavedSearch(ctx, uid)
}

func (s *Service) RecentRuns(ctx context.Context, limit int32) ([]dto.SourceRunDto, error) {
	rows, err := s.q.RecentRunsJoined(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SourceRunDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.SourceRunDto{
			ID:         dbutil.UUIDString(r.ID),
			SourceKey:  r.SourceKey,
			SearchID:   r.SearchID,
			StartedAt:  dbutil.Timestamp(r.StartedAt),
			FinishedAt: dbutil.TimestampPtr(r.FinishedAt),
			OK:         r.Ok,
			Found:      int(r.Found),
			New:        int(r.New),
			Error:      r.Error,
		})
	}
	return out, nil
}

func savedSearchDto(r sqlcgen.SavedSearch) dto.SavedSearchDto {
	var q dto.SearchQuery
	_ = dbutil.UnmarshalJSONB(r.Query, &q)
	return dto.SavedSearchDto{
		ID:        dbutil.UUIDString(r.ID),
		Name:      r.Name,
		Query:     q,
		Cron:      r.Cron,
		Enabled:   r.Enabled,
		LastRunAt: dbutil.TimestampPtr(r.LastRunAt),
	}
}

// ---------------------------------------------------------------------------
// RunSearch: enqueue one ingest task per (search × enabled source)
// ---------------------------------------------------------------------------

func (s *Service) RunSearch(ctx context.Context, searchID string) ([]string, error) {
	uid, err := dbutil.ParseUUID(searchID)
	if err != nil {
		return nil, err
	}
	search, err := s.q.GetSavedSearch(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("search %s not found", searchID)
	}

	var query dto.SearchQuery
	_ = dbutil.UnmarshalJSONB(search.Query, &query)

	allSources, err := s.sources.List(ctx)
	if err != nil {
		return nil, err
	}
	enabledKeys := make(map[string]bool, len(allSources))
	for _, src := range allSources {
		if src.Enabled {
			enabledKeys[src.Key] = true
		}
	}

	wanted := query.Sources
	if len(wanted) == 0 {
		wanted = s.registry.Keys()
	}
	keys := make([]string, 0, len(wanted))
	for _, k := range wanted {
		if enabledKeys[k] {
			keys = append(keys, k)
		}
	}

	// Claim the slot up front. If an enqueue below fails partway through, the
	// error propagates but the sources already queued stay queued — leaving
	// lastRunAt untouched would make the next scheduler tick see the search
	// as still due and scrape those sources a second time.
	if err := s.q.TouchSavedSearchLastRun(ctx, uid); err != nil {
		return nil, err
	}

	for _, key := range keys {
		label := key + " scrape"
		if search.Name != "" {
			label = search.Name + " — " + key
		}
		rec := activity.New(ctx, s.q, "ingest", label, nil, &key, "")

		var activityID *string
		if rec != nil {
			id := dbutil.UUIDString(rec.ID())
			activityID = &id
		}

		payload, err := json.Marshal(queue.IngestPayload{SearchID: &searchID, SourceKey: key, ActivityID: activityID})
		if err != nil {
			return nil, err
		}
		opts := []asynq.Option{asynq.MaxRetry(queue.IngestMaxRetry), asynq.Queue(queue.QueueIngest)}
		if activityID != nil {
			opts = append(opts, asynq.TaskID(*activityID))
		}
		if _, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload), opts...); err != nil {
			return nil, fmt.Errorf("ingestion: enqueue %s: %w", key, err)
		}
	}

	return keys, nil
}

// ---------------------------------------------------------------------------
// RunSource: enqueue one ingest task for a source (no search / subscription)
// ---------------------------------------------------------------------------

// RunSource enqueues an ingest task that scrapes a source with no saved search
// or subscription — a direct "run this source" trigger. Mirrors the planned
// POST /api/sources/{key}/run endpoint from plan/02-djinni-subscriptions.md.
func (s *Service) RunSource(ctx context.Context, sourceKey string) error {
	source, err := s.sources.GetByKey(ctx, sourceKey)
	if err != nil {
		return err
	}
	if !source.Enabled {
		return fmt.Errorf("source '%s' is disabled", sourceKey)
	}

	rec := activity.New(ctx, s.q, "ingest", sourceKey+" scrape", nil, &sourceKey, "")
	var activityID *string
	if rec != nil {
		id := dbutil.UUIDString(rec.ID())
		activityID = &id
	}

	payload, err := json.Marshal(queue.IngestPayload{SourceKey: sourceKey, ActivityID: activityID})
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.MaxRetry(queue.IngestMaxRetry), asynq.Queue(queue.QueueIngest)}
	if activityID != nil {
		opts = append(opts, asynq.TaskID(*activityID))
	}
	if _, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload), opts...); err != nil {
		return fmt.Errorf("ingestion: enqueue source %s: %w", sourceKey, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// RunSubscription: enqueue one ingest task for a URL-based subscription
// ---------------------------------------------------------------------------

// RunSubscription enqueues an ingest task that scrapes a single subscription
// URL through its source's adapter (SubscriptionURL in the query, instead of
// keywords). lastRunAt is touched by the ingest handler once the run finishes.
func (s *Service) RunSubscription(ctx context.Context, subscriptionID string) error {
	uid, err := dbutil.ParseUUID(subscriptionID)
	if err != nil {
		return err
	}
	sub, err := s.q.GetSubscription(ctx, uid)
	if err != nil {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}
	if !sub.Enabled {
		return fmt.Errorf("subscription %s is disabled", subscriptionID)
	}

	source, err := s.sources.GetByKey(ctx, sub.SourceKey)
	if err != nil {
		return err
	}
	if !source.Enabled {
		return fmt.Errorf("source '%s' is disabled", sub.SourceKey)
	}

	label := sub.SourceKey + " scrape"
	if sub.Name != nil && *sub.Name != "" {
		label = *sub.Name + " — " + sub.SourceKey
	}
	rec := activity.New(ctx, s.q, "ingest", label, nil, &sub.SourceKey, "")
	var activityID *string
	if rec != nil {
		id := dbutil.UUIDString(rec.ID())
		activityID = &id
	}

	payload, err := json.Marshal(queue.IngestPayload{SubscriptionID: &subscriptionID, SourceKey: sub.SourceKey, ActivityID: activityID})
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.MaxRetry(queue.IngestMaxRetry), asynq.Queue(queue.QueueIngest)}
	if activityID != nil {
		opts = append(opts, asynq.TaskID(*activityID))
	}
	if _, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload), opts...); err != nil {
		return fmt.Errorf("ingestion: enqueue subscription %s: %w", subscriptionID, err)
	}
	return nil
}

// RunAllSubscriptions enqueues an ingest task for every enabled subscription,
// skipping subscriptions whose source is disabled. Returns the number queued.
func (s *Service) RunAllSubscriptions(ctx context.Context) (int, error) {
	subs, err := s.q.ListSubscriptions(ctx)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}
		id := dbutil.UUIDString(sub.ID)
		if err := s.RunSubscription(ctx, id); err != nil {
			continue
		}
		queued++
	}
	return queued, nil
}
