package application

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/robfig/cron/v3"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/subscriptions/domain"
)

type Service struct {
	q       domain.Repository
	sources domain.SourceEnsurer
}

func NewService(q domain.Repository, sources domain.SourceEnsurer) *Service {
	return &Service{q: q, sources: sources}
}

func (s *Service) List(ctx context.Context) ([]dto.SubscriptionDto, error) {
	rows, err := s.q.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	return s.mapSubscriptions(ctx, rows), nil
}

func (s *Service) ListBySource(ctx context.Context, sourceKey string) ([]dto.SubscriptionDto, error) {
	rows, err := s.q.ListSubscriptionsBySource(ctx, sourceKey)
	if err != nil {
		return nil, err
	}
	return s.mapSubscriptions(ctx, rows), nil
}

const DefaultCron = "0 */6 * * *"

const (
	KindCrawl  = "crawl"
	KindManual = "manual"
)

func (s *Service) EnsureManualSubscription(ctx context.Context, sourceKey string) (sqlcgen.Subscription, error) {
	if sourceKey == "" {
		return sqlcgen.Subscription{}, apperr.Validation("sourceKey is required")
	}
	if _, err := s.sources.GetByKey(ctx, sourceKey); err != nil {
		return sqlcgen.Subscription{}, apperr.Validation(fmt.Sprintf("source '%s' not found", sourceKey))
	}
	return s.q.EnsureManualSubscription(ctx, sqlcgen.EnsureManualSubscriptionParams{
		SourceKey: sourceKey,
		Cron:      DefaultCron,
	})
}

func (s *Service) Create(ctx context.Context, sourceKey, rawURL string, name *string, enabled bool, cron, kind string) (*dto.SubscriptionDto, error) {
	if kind == KindManual {
		return nil, apperr.Validation("manual subscriptions are created implicitly by the first manual add, not through this endpoint")
	}
	if kind != "" && kind != KindCrawl {
		return nil, apperr.Validation(fmt.Sprintf("unknown subscription kind %q", kind))
	}
	if sourceKey == "" || rawURL == "" {
		return nil, apperr.Validation("sourceKey and url are required")
	}
	if cron == "" {
		cron = DefaultCron
	}
	if err := validateCron(cron); err != nil {
		return nil, err
	}
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
		Kind:      KindCrawl,
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
	current, err := s.q.GetSubscription(ctx, uid)
	if err != nil {
		return nil, err
	}
	if current.Kind == KindManual {

		if in.URL != nil {
			return nil, apperr.Validation("a manual subscription has no url to change")
		}
		if in.Cron != nil {
			return nil, apperr.Validation("a manual subscription is never scheduled, so its cron cannot be changed")
		}
		if in.Enabled != nil && !*in.Enabled {
			if err := s.refuseWhileVacanciesAttached(ctx, uid, "disabled"); err != nil {
				return nil, err
			}
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
	current, err := s.q.GetSubscription(ctx, uid)
	if err != nil {
		return err
	}
	if current.Kind == KindManual {
		if err := s.refuseWhileVacanciesAttached(ctx, uid, "deleted"); err != nil {
			return err
		}
	}
	return s.q.DeleteSubscription(ctx, uid)
}

func (s *Service) refuseWhileVacanciesAttached(ctx context.Context, id pgtype.UUID, verb string) error {
	count, err := s.q.CountJobsForSubscription(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperr.Conflict(fmt.Sprintf("this manual subscription has %d vacancies attached and cannot be %s", count, verb))
	}
	return nil
}

func validateCron(expr string) error {
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return nil
}

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
	case "djinni":
		return validateDjinniSubscriptionURL(rawURL)
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

func validateDjinniSubscriptionURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("djinni subscription url %q is not a valid URL", rawURL)
	}
	host := strings.ToLower(parsed.Host)
	if host != "djinni.co" && host != "www.djinni.co" {
		return fmt.Errorf("djinni subscription url %q must be a djinni.co url", rawURL)
	}
	if (parsed.Path == "/jobs" || parsed.Path == "/jobs/") &&
		parsed.Query().Get("search_type") == "basic-search" {
		return nil
	}
	return fmt.Errorf("djinni subscriptions support only preset-search URLs (`djinni.co/jobs/?search_type=basic-search&…`); dashboard URLs are no longer supported")
}

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

func (s *Service) mapSubscriptions(ctx context.Context, rows []sqlcgen.Subscription) []dto.SubscriptionDto {
	out := make([]dto.SubscriptionDto, 0, len(rows))
	for _, r := range rows {
		item := subscriptionDto(r)
		if r.Kind == KindManual {
			s.attachManualStats(ctx, r.ID, &item)
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) attachManualStats(ctx context.Context, id pgtype.UUID, item *dto.SubscriptionDto) {
	stats, err := s.q.ManualSubscriptionStats(ctx, id)
	if err != nil {
		return
	}
	count := int(stats.Added)
	item.ManualCount = &count
	item.LastAddedAt = dbutil.TimestampPtr(stats.LastAddedAt)
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
		Kind:      r.Kind,
	}
}
