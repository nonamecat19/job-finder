package adapters

import (
	"context"
	"fmt"
	"sync"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/jobsources/roster"
)

// employerFetcher reads one employer's open postings from a board vendor,
// returning the HTTP status code (0 if the request itself failed before any
// response) alongside the normalized jobs — the status is what lets
// runBoardVendor distinguish "not found" from "refused" from "unreadable"
// (FR-020). Takes the full roster row (not just the identifier) so the
// fetcher can stamp Company from the roster's display name — board vendor
// APIs don't reliably echo the employer's name back in the posting payload.
type employerFetcher func(ctx context.Context, employer sqlcgen.EmployerBoard) (statusCode int, jobs []dto.NormalizedJob, err error)

// boardRunState is embedded (by pointer) in every ATS board adapter to
// satisfy domain.EmployerReporter. A mutex guards it because the
// registry holds one shared adapter instance that asynq may drive from
// multiple worker goroutines.
type boardRunState struct {
	mu     sync.Mutex
	detail []domain.EmployerRunOutcome
}

func (b *boardRunState) LastRunDetail() []domain.EmployerRunOutcome {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]domain.EmployerRunOutcome, len(b.detail))
	copy(out, b.detail)
	return out
}

func (b *boardRunState) setDetail(d []domain.EmployerRunOutcome) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.detail = d
}

// runBoardVendor fans a single Search call out over every roster employer
// registered for vendor (research.md §2), capped by roster.ListForRun and
// roster.MaxPostingsPerEmployer (FR-007), isolating one employer's failure
// from the rest (FR-019) and classifying each outcome per FR-020. It records
// each employer's outcome via rosterSvc.RecordRunOutcome (feeding FR-014's
// stale flag) and returns an error only when FR-021's bar is hit: zero
// employers read successfully out of a non-empty roster.
func runBoardVendor(ctx context.Context, rosterSvc *roster.Service, state *boardRunState, vendor string, fetch employerFetcher) ([]dto.NormalizedJob, error) {
	employers, err := rosterSvc.ListForRun(ctx, vendor)
	if err != nil {
		return nil, fmt.Errorf("atsboard: %s: list roster: %w", vendor, err)
	}

	var jobs []dto.NormalizedJob
	detail := make([]domain.EmployerRunOutcome, 0, len(employers))
	successCount := 0

	for _, e := range employers {
		status, found, ferr := fetch(ctx, e)
		outcome := classifyOutcome(status, found, ferr)
		if outcome == domain.EmployerOutcomeRead || outcome == domain.EmployerOutcomeNoPostings {
			successCount++
		}
		if len(found) > roster.MaxPostingsPerEmployer {
			found = found[:roster.MaxPostingsPerEmployer]
		}
		if ferr == nil {
			jobs = append(jobs, found...)
		}
		detail = append(detail, domain.EmployerRunOutcome{
			EmployerIdentifier: e.EmployerIdentifier,
			Outcome:            outcome,
			PostingsFound:      len(found),
		})
		if err := rosterSvc.RecordRunOutcome(ctx, dbutil.UUIDString(e.ID), len(found)); err != nil {
			// Bookkeeping failure must not abort other employers' reads.
			continue
		}
	}

	state.setDetail(detail)

	if len(employers) > 0 && successCount == 0 {
		return jobs, fmt.Errorf("atsboard: %s: 0 of %d employers read successfully", vendor, len(employers))
	}
	return jobs, nil
}

func classifyOutcome(status int, jobs []dto.NormalizedJob, err error) domain.EmployerOutcome {
	switch {
	case err == nil:
		if len(jobs) == 0 {
			return domain.EmployerOutcomeNoPostings
		}
		return domain.EmployerOutcomeRead
	case status == 404:
		return domain.EmployerOutcomeNotFound
	case status == 401 || status == 403 || status == 429:
		return domain.EmployerOutcomeRefused
	default:
		return domain.EmployerOutcomeUnreadable
	}
}

// healthCheckEmployer is the shared roster.EmployerHealthChecker shape every
// board vendor's HealthCheck/RegisterFromURL validation uses: one read, no
// roster bookkeeping side effect.
func healthCheckEmployer(fetch employerFetcher) roster.EmployerHealthChecker {
	return func(ctx context.Context, employerIdentifier string) (int, error) {
		_, jobs, err := fetch(ctx, sqlcgen.EmployerBoard{EmployerIdentifier: employerIdentifier, DisplayName: employerIdentifier})
		if err != nil {
			return 0, err
		}
		return len(jobs), nil
	}
}

// vendorHealthCheck probes the first roster employer for vendor as the
// Adapter.HealthCheck implementation (FR-022's per-vendor health check on
// the Sources screen); an empty roster has nothing to fail, so it reports
// healthy.
func vendorHealthCheck(ctx context.Context, rosterSvc *roster.Service, vendor string, fetch employerFetcher) (bool, error) {
	employers, err := rosterSvc.ListForRun(ctx, vendor)
	if err != nil {
		return false, err
	}
	if len(employers) == 0 {
		return true, nil
	}
	status, _, err := fetch(ctx, employers[0])
	if err != nil {
		return false, fmt.Errorf("atsboard: %s: health check on %s failed (status %d): %w", vendor, employers[0].EmployerIdentifier, status, err)
	}
	return true, nil
}

// NewBoardAdapters constructs the five ATS board vendor adapters (013) with
// no roster.Service yet — callers must set each adapter's Roster field
// before wiring the registry, then pass the returned checkers map into
// roster.NewService so RegisterFromURL/Accept can health-check a pasted URL
// against the matching vendor before it joins the roster (FR-011). Adapter
// method values close over the pointer, not a copy, so setting Roster after
// this call still reaches the checker.
func NewBoardAdapters() (
	gh *GreenhouseAdapter,
	lv *LeverAdapter,
	as *AshbyAdapter,
	wk *WorkableAdapter,
	sr *SmartRecruitersAdapter,
	checkers map[string]roster.EmployerHealthChecker,
) {
	gh = &GreenhouseAdapter{}
	lv = &LeverAdapter{}
	as = &AshbyAdapter{}
	wk = &WorkableAdapter{}
	sr = &SmartRecruitersAdapter{}
	checkers = map[string]roster.EmployerHealthChecker{
		"greenhouse":      healthCheckEmployer(gh.fetchEmployer),
		"lever":           healthCheckEmployer(lv.fetchEmployer),
		"ashby":           healthCheckEmployer(as.fetchEmployer),
		"workable":        healthCheckEmployer(wk.fetchEmployer),
		"smartrecruiters": healthCheckEmployer(sr.fetchEmployer),
	}
	return gh, lv, as, wk, sr, checkers
}
