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

type employerFetcher func(ctx context.Context, employer sqlcgen.EmployerBoard) (statusCode int, jobs []dto.NormalizedJob, err error)

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

func healthCheckEmployer(fetch employerFetcher) roster.EmployerHealthChecker {
	return func(ctx context.Context, employerIdentifier string) (int, error) {
		_, jobs, err := fetch(ctx, sqlcgen.EmployerBoard{EmployerIdentifier: employerIdentifier, DisplayName: employerIdentifier})
		if err != nil {
			return 0, err
		}
		return len(jobs), nil
	}
}

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
