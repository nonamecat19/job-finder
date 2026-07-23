// Package autogen holds the "auto-generate resume on high match score"
// setting: a single enabled/threshold pair, configurable from Settings, and
// read by matching/handler.go right after a job's score is computed to
// decide whether to auto-enqueue a resume for it.
package autogen

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetAutoGenerateSetting(ctx context.Context) (sqlcgen.AutoGenerateSetting, error)
	UpdateAutoGenerateSetting(ctx context.Context, arg sqlcgen.UpdateAutoGenerateSettingParams) (sqlcgen.AutoGenerateSetting, error)
}

type State struct {
	Enabled   bool
	Threshold int
}

// Service caches the current setting in memory so the per-match hook
// (matching/handler.go) never needs a DB round trip on the hot path.
type Service struct {
	q       Repository
	current atomic.Pointer[State]
}

func NewService(ctx context.Context, q Repository) (*Service, error) {
	row, err := q.GetAutoGenerateSetting(ctx)
	if err != nil {
		return nil, fmt.Errorf("autogen: load: %w", err)
	}
	s := &Service{q: q}
	s.current.Store(&State{Enabled: row.Enabled, Threshold: int(row.Threshold)})
	return s, nil
}

func (s *Service) Get() State {
	return *s.current.Load()
}

func (s *Service) Update(ctx context.Context, enabled bool, threshold int) (State, error) {
	row, err := s.q.UpdateAutoGenerateSetting(ctx, sqlcgen.UpdateAutoGenerateSettingParams{
		Enabled:   enabled,
		Threshold: int32(threshold),
	})
	if err != nil {
		return State{}, fmt.Errorf("autogen: update: %w", err)
	}
	st := State{Enabled: row.Enabled, Threshold: int(row.Threshold)}
	s.current.Store(&st)
	return st, nil
}

// ShouldGenerate reports whether a job with this score should get an
// auto-enqueued resume, per the current cached setting.
func (s *Service) ShouldGenerate(score int) bool {
	st := s.Get()
	return st.Enabled && score >= st.Threshold
}
