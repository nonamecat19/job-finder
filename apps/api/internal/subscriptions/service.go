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
	"net/url"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// SourceEnsurer and Repository ports live in ports.go.
type Service struct {
	q       Repository
	sources SourceEnsurer
}

func NewService(q Repository, sources SourceEnsurer) *Service {
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

func (s *Service) Create(ctx context.Context, sourceKey, rawURL string, name *string, enabled bool) (*dto.SubscriptionDto, error) {
	if sourceKey == "" || rawURL == "" {
		return nil, fmt.Errorf("sourceKey and url are required")
	}
	// Validate the source against the code-defined registry (not the db) and
	// ensure its JobSource row exists so the FK is satisfied.
	if _, err := s.sources.GetByKey(ctx, sourceKey); err != nil {
		return nil, fmt.Errorf("source '%s' not found", sourceKey)
	}
	if err := validateSubscriptionURL(sourceKey, rawURL); err != nil {
		return nil, err
	}
	row, err := s.q.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{
		SourceKey: sourceKey,
		Name:      name,
		Url:       rawURL,
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

// validateSubscriptionURL rejects a subscription URL that can't belong to
// its declared source, at save time rather than at run time (FR-016 for
// Indeed, FR-015 for RemoteOK). Other sources are unchecked.
func validateSubscriptionURL(sourceKey, rawURL string) error {
	switch sourceKey {
	case "indeed":
		return validateIndeedSubscriptionURL(rawURL)
	case "remoteok":
		return validateRemoteOKSubscriptionURL(rawURL)
	default:
		return nil
	}
}

func validateIndeedSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("indeed subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "indeed.com" && !strings.HasSuffix(host, ".indeed.com") {
		return fmt.Errorf("indeed subscription url %q must be an indeed.com search url", rawURL)
	}
	if strings.Contains(parsed.Path, "/viewjob") || strings.Contains(parsed.Path, "/rc/clk") {
		return fmt.Errorf("indeed subscription url %q looks like a single job posting, not a search results page", rawURL)
	}
	return nil
}

func validateRemoteOKSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("remoteok subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "remoteok.com" && !strings.HasSuffix(host, ".remoteok.com") &&
		host != "remoteok.io" && !strings.HasSuffix(host, ".remoteok.io") {
		return fmt.Errorf("remoteok subscription url %q must be a remoteok.com or remoteok.io url", rawURL)
	}
	return nil
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
