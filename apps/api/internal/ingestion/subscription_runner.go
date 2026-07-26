package ingestion

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/queue"
)

// RunSubscription enqueues an ingest task for a URL-based subscription.
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

	payload, err := json.Marshal(queue.IngestPayload{
		SubscriptionID: &subscriptionID, SourceKey: sub.SourceKey, ActivityID: activityID,
	})
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

// RunAllSubscriptions enqueues ingest tasks for every enabled subscription.
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
