// Package application orchestrates the company-intel scrapers and the
// Company/CompanySignal persistence, serving the two card endpoints (GET
// intel, POST refresh).
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/companyintel/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// ErrNoCompany re-exports domain.ErrNoCompany for callers that only import
// application (e.g. the httpapi handler).
var ErrNoCompany = domain.ErrNoCompany

// Service orchestrates the company-intel scrapers and the Company /
// CompanySignal persistence, serving the two card endpoints (GET intel,
// POST refresh).
type Service struct {
	q        domain.Repository
	registry *domain.Registry
	// domainPace is the minimum interval between two requests to the same
	// external domain (FR-012), defaulting to 2s in NewService.
	domainPace time.Duration
}

func NewService(q domain.Repository, registry *domain.Registry, domainPace time.Duration) *Service {
	if domainPace <= 0 {
		domainPace = 2 * time.Second
	}
	return &Service{q: q, registry: registry, domainPace: domainPace}
}

// GetIntel returns the currently-cached signal set for the job's company,
// or (nil, nil) when the company has never been probed (or the job has no
// parseable company name) — the handler renders that as the card's
// "no data yet" state rather than an error.
func (s *Service) GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("companyintel: job %s not found: %w", jobID, err)
	}

	normalized := domain.NormalizeCompanyName(job.Company)
	if normalized == "" {
		return nil, nil
	}

	company, err := s.q.GetCompanyByNormalizedName(ctx, normalized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("companyintel: lookup company %q: %w", normalized, err)
	}

	signals, err := s.q.GetCompanySignals(ctx, company.ID)
	if err != nil {
		return nil, fmt.Errorf("companyintel: list signals for %q: %w", normalized, err)
	}
	if len(signals) == 0 {
		return nil, nil
	}

	out := buildDto(company, signals)
	return &out, nil
}

// Refresh re-scrapes every registered signal source for the job's company
// and upserts the results, then returns the (now-fresh, or partially
// fresh) signal set. Each source fails independently (FR-006): a failed
// source leaves its previously-cached CompanySignal row untouched. If
// every source fails, the returned Dto still carries any previously-cached
// values plus a top-level Error (FR-007).
func (s *Service) Refresh(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("companyintel: job %s not found: %w", jobID, err)
	}

	normalized := domain.NormalizeCompanyName(job.Company)
	if normalized == "" {
		return nil, domain.ErrNoCompany
	}

	company, err := s.q.UpsertCompany(ctx, sqlcgen.UpsertCompanyParams{
		Name:           strings.TrimSpace(job.Company),
		NormalizedName: normalized,
		Website:        nil,
	})
	if err != nil {
		return nil, fmt.Errorf("companyintel: upsert company %q: %w", normalized, err)
	}

	website := ""
	if company.Website != nil {
		website = *company.Website
	}

	var previousHeadcount *int
	if prev, err := s.q.GetCompanySignalByKind(ctx, sqlcgen.GetCompanySignalByKindParams{
		CompanyId: company.ID,
		Kind:      domain.KindHeadcount,
	}); err == nil {
		if n, ok := domain.ParseLeadingInt(prev.Value); ok {
			previousHeadcount = &n
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("companyintel: read previous headcount failed", "company", normalized, "error", err)
	}

	in := domain.Input{
		CompanyName:       strings.TrimSpace(job.Company),
		Website:           website,
		JobDescription:    job.Description,
		PreviousHeadcount: previousHeadcount,
	}

	succeeded := s.runScrapers(ctx, company.ID, in)

	if err := s.q.UpdateCompanyLastRefreshed(ctx, company.ID); err != nil {
		slog.Warn("companyintel: update lastRefreshedAt failed", "company", normalized, "error", err)
	}

	signals, err := s.q.GetCompanySignals(ctx, company.ID)
	if err != nil {
		return nil, fmt.Errorf("companyintel: list signals for %q: %w", normalized, err)
	}

	out := buildDto(company, signals)
	if succeeded == 0 && len(s.registry.All()) > 0 {
		msg := "all company-intel sources were unreachable; showing previously cached data"
		out.Error = &msg
	}
	return &out, nil
}

// runScrapers runs every registered scraper, one domain-group per
// goroutine (so distinct domains run concurrently) with domainPace
// enforced between requests within a domain group (FR-012). It returns the
// count of scrapers that produced a signal (success or deliberate
// zero-result) that was persisted.
func (s *Service) runScrapers(ctx context.Context, companyID pgtype.UUID, in domain.Input) int {
	var succeeded int32
	var wg sync.WaitGroup
	var mu sync.Mutex

	for group, scrapers := range s.registry.ByDomain() {
		wg.Add(1)
		go func(group string, scrapers []domain.Scraper) {
			defer wg.Done()
			for i, sc := range scrapers {
				if i > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(s.domainPace):
					}
				}

				result, err := sc.Scrape(ctx, in)
				if err != nil {
					slog.Warn("companyintel: scrape failed",
						"domain", group, "kind", sc.Kind(), "company", in.CompanyName, "error", err)
					continue
				}
				if result == nil {
					// Deliberate skip (e.g. no website known yet) — not a
					// failure, nothing to persist.
					continue
				}

				if err := s.persistSignal(ctx, companyID, *result); err != nil {
					slog.Warn("companyintel: persist signal failed",
						"domain", group, "kind", sc.Kind(), "company", in.CompanyName, "error", err)
					continue
				}

				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}(group, scrapers)
	}

	wg.Wait()
	return int(succeeded)
}

func (s *Service) persistSignal(ctx context.Context, companyID pgtype.UUID, result domain.SignalResult) error {
	valueJSON, err := json.Marshal(result.Value)
	if err != nil {
		return fmt.Errorf("marshal value: %w", err)
	}
	var rawJSON []byte
	if result.Raw != nil {
		rawJSON, err = json.Marshal(result.Raw)
		if err != nil {
			return fmt.Errorf("marshal raw: %w", err)
		}
	}
	var source *string
	if result.Source != "" {
		source = &result.Source
	}

	_, err = s.q.UpsertCompanySignal(ctx, sqlcgen.UpsertCompanySignalParams{
		CompanyId: companyID,
		Kind:      result.Kind,
		Value:     valueJSON,
		Source:    source,
		Raw:       rawJSON,
	})
	return err
}

// buildDto flattens a Company row and its CompanySignal rows into the wire
// DTO. FetchedAt is the most recent of the individual signal timestamps.
func buildDto(company sqlcgen.Company, signals []sqlcgen.CompanySignal) dto.CompanyIntelDto {
	out := dto.CompanyIntelDto{
		CompanyName: company.Name,
		Website:     company.Website,
	}

	var latest time.Time
	for _, sig := range signals {
		if sig.FetchedAt.Valid && sig.FetchedAt.Time.After(latest) {
			latest = sig.FetchedAt.Time
		}

		switch sig.Kind {
		case domain.KindFunding:
			if v, ok := domain.DecodeString(sig.Value); ok {
				out.Funding = &v
			}
		case domain.KindLayoffs:
			if v, ok := domain.DecodeString(sig.Value); ok {
				out.Layoffs = &v
			}
		case domain.KindGlassdoorRating:
			var v float64
			if err := json.Unmarshal(sig.Value, &v); err == nil {
				out.GlassdoorRating = &v
			}
		case domain.KindHeadcount:
			if v, ok := domain.DecodeString(sig.Value); ok {
				out.Headcount = &v
			}
		case domain.KindTechStack:
			if v, ok := domain.DecodeString(sig.Value); ok {
				out.TechStack = &v
			}
		}
	}

	if !latest.IsZero() {
		out.FetchedAt = latest.UTC().Format(time.RFC3339)
	} else if company.LastRefreshedAt.Valid {
		out.FetchedAt = company.LastRefreshedAt.Time.UTC().Format(time.RFC3339)
	}

	return out
}
