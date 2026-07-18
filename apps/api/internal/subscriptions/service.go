// Package subscriptions manages URL-based subscriptions: a saved-filter URL on
// a job site attached to a JobSource by key. This pass is CRUD + enable toggle
// only; fetching/scraping each URL is deferred.
//
// TODO(subscriptions): add an adapter method to fetch a subscription URL and a
// scheduler that iterates enabled subscriptions and touches "lastRunAt" —
// integrate at internal/ingestion/scheduler.go, mirroring the SavedSearch flow.
package subscriptions

import (
	"context"
	"fmt"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// SourceEnsurer validates a source key against the code-defined adapter
// registry and lazily materializes its JobSource row (needed to satisfy the
// Subscription -> JobSource FK). Source identity is hardcoded in the registry,
// not seeded in the db, so this is the single point that turns a key into a row.
type SourceEnsurer interface {
	GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error)
}

type Service struct {
	q       *sqlcgen.Queries
	sources SourceEnsurer
}

func NewService(q *sqlcgen.Queries, sources SourceEnsurer) *Service {
	return &Service{q: q, sources: sources}
}

func (s *Service) List(ctx context.Context) ([]dto.SubscriptionDto, error) {
	rows, err := s.q.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	return mapSubscriptions(rows), nil
}

func (s *Service) ListBySource(ctx context.Context, sourceKey string) ([]dto.SubscriptionDto, error) {
	rows, err := s.q.ListSubscriptionsBySource(ctx, sourceKey)
	if err != nil {
		return nil, err
	}
	return mapSubscriptions(rows), nil
}

func (s *Service) Create(ctx context.Context, sourceKey, url string, name *string, enabled bool) (*dto.SubscriptionDto, error) {
	if sourceKey == "" || url == "" {
		return nil, fmt.Errorf("sourceKey and url are required")
	}
	// Validate the source against the code-defined registry (not the db) and
	// ensure its JobSource row exists so the FK is satisfied.
	if _, err := s.sources.GetByKey(ctx, sourceKey); err != nil {
		return nil, fmt.Errorf("source '%s' not found", sourceKey)
	}
	row, err := s.q.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{
		SourceKey: sourceKey,
		Name:      name,
		Url:       url,
		Enabled:   enabled,
	})
	if err != nil {
		return nil, err
	}
	out := subscriptionDto(row)
	return &out, nil
}

type UpdateInput struct {
	Name    *string
	URL     *string
	Enabled *bool
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*dto.SubscriptionDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return nil, err
	}
	row, err := s.q.UpdateSubscription(ctx, sqlcgen.UpdateSubscriptionParams{
		ID:      uid,
		Name:    in.Name,
		Url:     in.URL,
		Enabled: in.Enabled,
	})
	if err != nil {
		return nil, err
	}
	out := subscriptionDto(row)
	return &out, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.q.DeleteSubscription(ctx, uid)
}

func mapSubscriptions(rows []sqlcgen.Subscription) []dto.SubscriptionDto {
	out := make([]dto.SubscriptionDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, subscriptionDto(r))
	}
	return out
}

func subscriptionDto(r sqlcgen.Subscription) dto.SubscriptionDto {
	return dto.SubscriptionDto{
		ID:        dbutil.UUIDString(r.ID),
		SourceKey: r.SourceKey,
		Name:      r.Name,
		URL:       r.Url,
		Enabled:   r.Enabled,
		LastRunAt: dbutil.TimestampPtr(r.LastRunAt),
	}
}
