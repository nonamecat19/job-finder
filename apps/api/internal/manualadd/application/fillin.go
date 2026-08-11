package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/manualadd/domain"
	jsadapter "github.com/nonamecat19/jobscraper/adapter"
	"github.com/nonamecat19/jobscraper/ports"
)

// FillIn is a vacancy the operator completed by hand. Nothing is fetched.
type FillIn struct {
	URL         string
	SourceKey   *string
	Title       string
	Company     string
	Description string
	Location    *string
	Remote      bool
	SalaryRaw   *string
	PostedAt    *string
}

// SaveFillIn stores a hand-completed vacancy through the same ingest path an
// extracted one takes, so the two are indistinguishable downstream (FR-009).
// postedAt is stored exactly as given, including not at all — never defaulted
// to now, which would make an unknown-age vacancy look fresh (FR-017a).
func (s *Service) SaveFillIn(ctx context.Context, in FillIn) (domain.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, AddTimeout)
	defer cancel()

	rawURL := strings.TrimSpace(in.URL)
	if _, failure := parseURL(rawURL); failure != nil {
		return domain.Result{Outcome: domain.OutcomeFailed, Kind: failure.Kind, Reason: failure.Reason}, nil
	}
	if missing := missingFillInFields(in); len(missing) > 0 {
		return domain.Result{}, &MissingFieldsError{Fields: missing}
	}

	// A host with no reader has no source of its own, so its vacancies hang off
	// the manual source (FR-012a).
	sourceKey := ManualSourceKey
	if in.SourceKey != nil && strings.TrimSpace(*in.SourceKey) != "" {
		sourceKey = strings.TrimSpace(*in.SourceKey)
	}

	source, err := s.sources.GetByKey(ctx, sourceKey)
	if err != nil {
		return domain.Result{}, fmt.Errorf("manualadd: resolve source %s: %w", sourceKey, err)
	}
	subscription, err := s.subs.EnsureManualSubscription(ctx, sourceKey)
	if err != nil {
		return domain.Result{}, fmt.Errorf("manualadd: ensure manual subscription: %w", err)
	}

	run, err := s.q.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId:       source.ID,
		SubscriptionId: subscription.ID,
		Trigger:        manualTrigger,
	})
	if err != nil {
		return domain.Result{}, fmt.Errorf("manualadd: insert source run: %w", err)
	}

	posting := dto.NormalizedJob{
		SourceKey:   sourceKey,
		Title:       strings.TrimSpace(in.Title),
		Company:     strings.TrimSpace(in.Company),
		Description: in.Description,
		URL:         rawURL,
		Location:    in.Location,
		Remote:      in.Remote,
		SalaryRaw:   in.SalaryRaw,
		PostedAt:    in.PostedAt,
		Raw:         map[string]any{},
	}

	needsDetail := false
	if adapter, err := s.adapterFor(sourceKey); err == nil {
		needsDetail = jsadapter.NeedsDetail(adapter)
	}

	result, err := s.persist(ctx, posting, subscription.ID, run.ID, needsDetail)
	if err != nil {
		failure := classifyReadError(ctx, sourceKey, err)
		s.finishRunError(ctx, run.ID, failure)
		return domain.Result{}, err
	}

	_ = s.q.TouchSubscriptionLastRun(ctx, subscription.ID)
	return result, nil
}

func (s *Service) adapterFor(sourceKey string) (ports.JobSource, error) {
	for _, adapter := range s.registry.All() {
		if adapter.Key() == sourceKey {
			return adapter, nil
		}
	}
	return nil, fmt.Errorf("manualadd: no adapter registered for %s", sourceKey)
}

// MissingFieldsError names every field the operator still has to supply, rather
// than stopping at the first (FR-020).
type MissingFieldsError struct {
	Fields []string
}

func (e *MissingFieldsError) Error() string {
	verb := " are required"
	if len(e.Fields) == 1 {
		verb = " is required"
	}
	return strings.Join(e.Fields, ", ") + verb
}

func missingFillInFields(in FillIn) []string {
	var missing []string
	if strings.TrimSpace(in.Title) == "" {
		missing = append(missing, "title")
	}
	if strings.TrimSpace(in.Company) == "" {
		missing = append(missing, "company")
	}
	if strings.TrimSpace(in.Description) == "" {
		missing = append(missing, "description")
	}
	return missing
}
