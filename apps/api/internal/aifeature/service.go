// Package aifeature holds the "run this AI feature automatically when a
// job's match score is high enough" settings: one enabled/threshold pair per
// feature, configurable from Settings, and read by matching/handler.go right
// after a job's score is computed to decide whether to auto-enqueue that
// feature's work. Below the threshold (or when disabled), the feature only
// ever runs on-demand. Match scoring itself has no entry here — it always
// runs unconditionally.
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

// Keys lists every configurable feature, in the order Settings displays them.
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

// Service caches the current settings in memory so the per-match hook
// (matching/handler.go) never needs a DB round trip on the hot path.
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

// Get returns the current setting for one feature (zero value if unknown).
func (s *Service) Get(feature string) State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current[feature]
}

// GetAll returns every feature's setting, in Keys order.
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

// ShouldRun reports whether a job with this score should trigger the given
// feature automatically, per its current cached setting. Unknown features
// report false.
func (s *Service) ShouldRun(feature string, score int) bool {
	st := s.Get(feature)
	return st.Enabled && score >= st.Threshold
}
