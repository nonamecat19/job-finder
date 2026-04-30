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

	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/jobsources"
	"github.com/job-finder/api-go/internal/queue"
)

type Service struct {
	q        *sqlcgen.Queries
	registry *jobsources.Registry
	client   *asynq.Client
}

func NewService(q *sqlcgen.Queries, registry *jobsources.Registry, client *asynq.Client) *Service {
	return &Service{q: q, registry: registry, client: client}
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

	enabledSources, err := s.q.ListEnabledJobSources(ctx)
	if err != nil {
		return nil, err
	}
	enabledKeys := make(map[string]bool, len(enabledSources))
	for _, src := range enabledSources {
		enabledKeys[src.Key] = true
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
		payload, err := json.Marshal(queue.IngestPayload{SearchID: &searchID, SourceKey: key})
		if err != nil {
			return nil, err
		}
		// attempts: 1 (no retry), matching ingestQueue.add's { attempts: 1 }.
		if _, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload), asynq.MaxRetry(0)); err != nil {
			return nil, fmt.Errorf("ingestion: enqueue %s: %w", key, err)
		}
	}

	if err := s.q.TouchSavedSearchLastRun(ctx, uid); err != nil {
		return nil, err
	}
	return keys, nil
}
