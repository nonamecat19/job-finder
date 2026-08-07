package aifeature

import (
	"context"
	"fmt"
	"sync"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

const (
	Resume      = "resume"
	CoverLetter = "cover_letter"
	SalaryInfer = "salary_infer"
)

var Keys = []string{Resume, CoverLetter, SalaryInfer}

type Repository interface {
	ListAiFeatureSettings(ctx context.Context) ([]sqlcgen.AiFeatureSetting, error)
	UpdateAiFeatureSetting(ctx context.Context, arg sqlcgen.UpdateAiFeatureSettingParams) (sqlcgen.AiFeatureSetting, error)
}

type State struct {
	Feature   string
	Enabled   bool
	Threshold int
}

type Service struct {
	q Repository

	mu      sync.RWMutex
	current map[string]State
}

func NewService(ctx context.Context, q Repository) (*Service, error) {
	rows, err := q.ListAiFeatureSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("aifeature: load: %w", err)
	}
	s := &Service{q: q, current: make(map[string]State, len(rows))}
	for _, r := range rows {
		s.current[r.FeatureKey] = State{Feature: r.FeatureKey, Enabled: r.Enabled, Threshold: int(r.Threshold)}
	}
	return s, nil
}

func (s *Service) Get(feature string) State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current[feature]
}

func (s *Service) GetAll() []State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]State, 0, len(Keys))
	for _, k := range Keys {
		out = append(out, s.current[k])
	}
	return out
}

func (s *Service) Update(ctx context.Context, feature string, enabled bool, threshold int) (State, error) {
	row, err := s.q.UpdateAiFeatureSetting(ctx, sqlcgen.UpdateAiFeatureSettingParams{
		FeatureKey: feature,
		Enabled:    enabled,
		Threshold:  int32(threshold),
	})
	if err != nil {
		return State{}, fmt.Errorf("aifeature: update: %w", err)
	}
	st := State{Feature: row.FeatureKey, Enabled: row.Enabled, Threshold: int(row.Threshold)}
	s.mu.Lock()
	s.current[feature] = st
	s.mu.Unlock()
	return st, nil
}

func (s *Service) ShouldRun(feature string, score int) bool {
	st := s.Get(feature)
	return st.Enabled && score >= st.Threshold
}
