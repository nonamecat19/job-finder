package application

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	jsadapter "github.com/nonamecat19/job-scraper/adapter"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/queue"
)

const subscriptionKindManual = "manual"

const (
	reconcileMinAge = 30 * time.Minute
	reconcileMaxAge = 7 * 24 * time.Hour
	reconcileBatch  = 50
)

type SearchService struct {
	q        domain.SearchRepository
	registry *jsadapter.Registry
	sources  *Service
	client   Enqueuer
}

func NewSearchService(q domain.SearchRepository, registry *jsadapter.Registry, sources *Service, client Enqueuer) *SearchService {
	return &SearchService{q: q, registry: registry, sources: sources, client: client}
}

func (s *SearchService) ListSearches(ctx context.Context) ([]dto.SavedSearchDto, error) {
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

func (s *SearchService) CreateSearch(ctx context.Context, name string, query dto.SearchQuery, cron string, enabled bool) (*dto.SavedSearchDto, error) {
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

func (s *SearchService) UpdateSearch(ctx context.Context, id string, in UpdateSearchInput) (*dto.SavedSearchDto, error) {
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

func (s *SearchService) DeleteSearch(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteSavedSearch(ctx, uid)
}

func (s *SearchService) RecentRuns(ctx context.Context, limit int32) ([]dto.SourceRunDto, error) {
	rows, err := s.q.RecentRunsJoined(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SourceRunDto, 0, len(rows))
	for _, r := range rows {
		var subscriptionID *string
		if r.SubscriptionID.Valid {
			id := dbutil.UUIDString(r.SubscriptionID)
			subscriptionID = &id
		}
		out = append(out, dto.SourceRunDto{
			ID:             dbutil.UUIDString(r.ID),
			SourceKey:      r.SourceKey,
			SearchID:       r.SearchID,
			SubscriptionID: subscriptionID,
			Trigger:        r.Trigger,
			StartedAt:      dbutil.Timestamp(r.StartedAt),
			FinishedAt:     dbutil.TimestampPtr(r.FinishedAt),
			OK:             r.Ok,
			Found:          int(r.Found),
			New:            int(r.New),
			Error:          r.Error,
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

func (s *SearchService) RunSearch(ctx context.Context, searchID string) ([]string, error) {
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
		if err := s.client.EnqueueContext(ctx, queue.TypeIngest, payload); err != nil {
			return nil, fmt.Errorf("ingestion: enqueue %s: %w", key, err)
		}
	}

	if err := s.q.TouchSavedSearchLastRun(ctx, uid); err != nil {
		return nil, err
	}
	return keys, nil
}

func (s *SearchService) RunSource(ctx context.Context, sourceKey string) error {
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
	if err := s.client.EnqueueContext(ctx, queue.TypeIngest, payload); err != nil {
		return fmt.Errorf("ingestion: enqueue source %s: %w", sourceKey, err)
	}
	return nil
}

func (s *SearchService) RunSubscription(ctx context.Context, subscriptionID string) error {
	uid, err := dbutil.ParseUUID(subscriptionID)
	if err != nil {
		return err
	}
	sub, err := s.q.GetSubscription(ctx, uid)
	if err != nil {
		return fmt.Errorf("subscription %s not found", subscriptionID)
	}
	if sub.Kind == subscriptionKindManual {
		return apperr.Validation("a manual subscription has nothing to crawl — it holds vacancies added by hand")
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
	if err := s.client.EnqueueContext(ctx, queue.TypeIngest, payload); err != nil {
		return fmt.Errorf("ingestion: enqueue subscription %s: %w", subscriptionID, err)
	}
	return nil
}

func (s *SearchService) RunAllSubscriptions(ctx context.Context) (int, error) {
	subs, err := s.q.ListSubscriptions(ctx)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, sub := range subs {
		if !sub.Enabled || sub.Kind == subscriptionKindManual {
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

func (s *SearchService) ReconcileUnmatched(ctx context.Context) (int, error) {
	now := time.Now()
	rows, err := s.q.ListJobsMissingMatch(ctx, sqlcgen.ListJobsMissingMatchParams{
		OlderThan: dbutil.TimestampAt(now.Add(-reconcileMinAge)),
		NewerThan: dbutil.TimestampAt(now.Add(-reconcileMaxAge)),
		Limit:     reconcileBatch,
	})
	if err != nil {
		return 0, err
	}

	queued := 0
	for _, row := range rows {
		jobID := dbutil.UUIDString(row.ID)

		var actID *string
		rec := activity.New(ctx, s.q, "match", fmt.Sprintf("%s — %s", row.Company, row.Title), &jobID, nil, "")
		if rec != nil {
			idStr := dbutil.UUIDString(rec.ID())
			actID = &idStr
		}

		payload, err := json.Marshal(queue.MatchPayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			continue
		}
		if err := s.client.EnqueueContext(ctx, queue.TypeMatch, payload); err != nil {
			slog.Warn("ingestion: reconcile enqueue match failed", "job", jobID, "error", err)
			continue
		}
		queued++
	}
	if queued > 0 {
		slog.Info("reconciled jobs with no match result", "count", queued)
	}
	return queued, nil
}
