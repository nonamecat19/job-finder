// Package subscriptions manages URL-based subscriptions: a saved-filter URL on
// a job site attached to a JobSource by key. CRUD plus the enable toggle and
// the cron the ingestion scheduler runs the subscription on.
package subscriptions

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/robfig/cron/v3"

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

// DefaultCron is the schedule a subscription gets when the caller doesn't
// pick one, matching the SavedSearch default and the column default.
const DefaultCron = "0 */6 * * *"

func (s *Service) Create(ctx context.Context, sourceKey, rawURL string, name *string, enabled bool, cron string) (*dto.SubscriptionDto, error) {
	if sourceKey == "" || rawURL == "" {
		return nil, fmt.Errorf("sourceKey and url are required")
	}
	if cron == "" {
		cron = DefaultCron
	}
	if err := validateCron(cron); err != nil {
		return nil, err
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
		Cron:      cron,
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
	Cron    *string
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*dto.SubscriptionDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return nil, err
	}
	if in.Cron != nil {
		if err := validateCron(*in.Cron); err != nil {
			return nil, err
		}
	}
	row, err := s.q.UpdateSubscription(ctx, sqlcgen.UpdateSubscriptionParams{
		ID:      uid,
		Name:    in.Name,
		Url:     in.URL,
		Enabled: in.Enabled,
		Cron:    in.Cron,
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

// validateCron rejects a cron expression the scheduler can't parse, at save
// time rather than leaving the subscription silently unschedulable: the
// scheduler logs and skips a bad expression, which looks like a subscription
// that simply never runs.
func validateCron(expr string) error {
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return nil
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
	case "glassdoor":
		return validateGlassdoorSubscriptionURL(rawURL)
	case "jobleads":
		return validateJobLeadsSubscriptionURL(rawURL)
	case "wellfound":
		return validateWellfoundSubscriptionURL(rawURL)
	case "himalayas":
		return validateHimalayasSubscriptionURL(rawURL)
	case "jobgether":
		return validateJobgetherSubscriptionURL(rawURL)
	default:
		return nil
	}
}

func validateHimalayasSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("himalayas subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "himalayas.app" && !strings.HasSuffix(host, ".himalayas.app") {
		return fmt.Errorf("himalayas subscription url %q must be a himalayas.app url", rawURL)
	}
	if parsed.Path != "/jobs" && !strings.HasPrefix(parsed.Path, "/jobs/") {
		return fmt.Errorf("himalayas subscription url %q must be a /jobs search url", rawURL)
	}
	if strings.TrimSpace(parsed.Query().Get("categories")) == "" {
		return fmt.Errorf("himalayas subscription url %q must include a 'categories' query parameter", rawURL)
	}
	return nil
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

func validateGlassdoorSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("glassdoor subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "glassdoor.com" && !strings.HasSuffix(host, ".glassdoor.com") {
		return fmt.Errorf("glassdoor subscription url %q must be a glassdoor.com search url", rawURL)
	}
	if strings.Contains(parsed.Path, "/job-listing/") {
		return fmt.Errorf("glassdoor subscription url %q looks like a single job posting, not a search results page", rawURL)
	}
	return nil
}

// validateWellfoundSubscriptionURL mirrors validateGlassdoorSubscriptionURL
// (specs/010-wellfound-job-provider/research.md R6). The legacy angel.co
// host is accepted alongside wellfound.com/*.wellfound.com since Wellfound
// was previously branded AngelList/angel.co and old saved-search links may
// still use it. A path shaped like a single job-detail page (e.g.
// "/jobs/12345-role-slug") is rejected in favor of a search-results page
// (e.g. "/role/r/golang-engineer").
var wellfoundJobDetailPathRe = regexp.MustCompile(`/jobs/\d`)

func validateWellfoundSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("wellfound subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "wellfound.com" && !strings.HasSuffix(host, ".wellfound.com") &&
		host != "angel.co" && !strings.HasSuffix(host, ".angel.co") {
		return fmt.Errorf("wellfound subscription url %q must be a wellfound.com (or legacy angel.co) search url", rawURL)
	}
	if wellfoundJobDetailPathRe.MatchString(parsed.Path) {
		return fmt.Errorf("wellfound subscription url %q looks like a single job posting, not a search results page", rawURL)
	}
	return nil
}

func validateJobLeadsSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("jobleads subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "jobleads.com" && !strings.HasSuffix(host, ".jobleads.com") {
		return fmt.Errorf("jobleads subscription url %q must be a jobleads.com search url", rawURL)
	}
	return nil
}

// validateJobgetherSubscriptionURL rejects a Jobgether subscription URL that
// isn't a jobgether.com search-results page, mirroring
// validateGlassdoorSubscriptionURL's host + shape check (FR-015, research.md
// R6). Jobgether search-results pages live under /jobs/search; a single
// job-detail page (e.g. /jobs/<slug>-<id>) is rejected with a human-readable
// reason.
func validateJobgetherSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("jobgether subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "jobgether.com" && !strings.HasSuffix(host, ".jobgether.com") {
		return fmt.Errorf("jobgether subscription url %q must be a jobgether.com search url", rawURL)
	}
	if strings.HasPrefix(parsed.Path, "/jobs/") && !strings.HasPrefix(parsed.Path, "/jobs/search") {
		return fmt.Errorf("jobgether subscription url %q looks like a single job posting, not a search results page", rawURL)
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
		Cron:      r.Cron,
		LastRunAt: dbutil.TimestampPtr(r.LastRunAt),
	}
}
