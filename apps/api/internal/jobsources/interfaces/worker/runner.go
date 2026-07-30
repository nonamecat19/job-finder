package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/queue"
)

// RunSearch enqueues one ingest task per (search × enabled source).
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

// RunSource enqueues an ingest task for a single source with no search or
// subscription — a direct "run this source" trigger.
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
