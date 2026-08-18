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

func NewService(ctx context.Context, q Repository) (*Service, error) {
	s := &Service{q: q, current: domain.DefaultSummaryOption()}
	row, err := q.GetSummaryModelSetting(ctx)
	if err != nil {
		return s, nil
	}

	opt, _ := domain.LookupSummaryOption(row.OptionId)
	s.current = opt
	return s, nil
}

func (s *Service) SummaryOption(context.Context) domain.SummaryOption { return s.Get() }

func (s *Service) Get() domain.SummaryOption {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

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

func (s *Service) Options() []domain.SummaryOption {
	return domain.SummaryOptions()
}
