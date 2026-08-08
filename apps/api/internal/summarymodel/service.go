// Package summarymodel stores which summary-writing option the user picked
// (034). It is modelled on resumeshape, which solved the same problem — a
// singleton user setting the generation pipeline reads through a narrow port —
// and solved it the right way round: the pipeline resolves the value once at
// the top of a run, and the port is declared by the generation package so that
// package never imports this one.
package summarymodel

import (
	"context"
	"fmt"
	"sync"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation/domain"
)

type Repository interface {
	GetSummaryModelSetting(ctx context.Context) (sqlcgen.SummaryModelSetting, error)
	UpdateSummaryModelSetting(ctx context.Context, optionID string) (sqlcgen.SummaryModelSetting, error)
}

type Service struct {
	q Repository

	mu      sync.RWMutex
	current domain.SummaryOption
}

// NewService loads the stored choice once at startup and caches it. A missing
// or unreadable row is not an error: the catalogue default is a complete
// answer, and refusing to start the API because a preferences table is empty
// would be a poor trade.
func NewService(ctx context.Context, q Repository) (*Service, error) {
	s := &Service{q: q, current: domain.DefaultSummaryOption()}
	row, err := q.GetSummaryModelSetting(ctx)
	if err != nil {
		return s, nil
	}
	// A stored id the catalogue no longer knows — an option removed between
	// releases — resolves to the default rather than propagating. The user sees
	// the default preselected next time they look, which is the truth.
	opt, _ := domain.LookupSummaryOption(row.OptionId)
	s.current = opt
	return s, nil
}

// SummaryOption satisfies the generation package's port.
func (s *Service) SummaryOption(context.Context) domain.SummaryOption { return s.Get() }

func (s *Service) Get() domain.SummaryOption {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Update validates the id against the catalogue and stores it. Unlike the read
// path, an unknown id here is rejected: a write is a user asserting something,
// and silently storing the default in place of what they asked for would make
// the selector lie back to them.
func (s *Service) Update(ctx context.Context, optionID string) (domain.SummaryOption, error) {
	opt, ok := domain.LookupSummaryOption(optionID)
	if !ok {
		return domain.SummaryOption{}, fmt.Errorf("summarymodel: unknown option %q", optionID)
	}
	row, err := s.q.UpdateSummaryModelSetting(ctx, opt.ID)
	if err != nil {
		return domain.SummaryOption{}, fmt.Errorf("summarymodel: update: %w", err)
	}
	stored, _ := domain.LookupSummaryOption(row.OptionId)
	s.mu.Lock()
	s.current = stored
	s.mu.Unlock()
	return stored, nil
}

func (s *Service) Reset(ctx context.Context) (domain.SummaryOption, error) {
	return s.Update(ctx, domain.DefaultSummaryOption().ID)
}

// Options is the menu, with the user's current choice marked. The dashboard
// renders this directly, so the marking happens here rather than being
// recomputed by every caller.
func (s *Service) Options() []domain.SummaryOption {
	return domain.SummaryOptions()
}
