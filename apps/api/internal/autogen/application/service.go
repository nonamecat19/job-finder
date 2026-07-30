package application

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/job-finder/api/internal/autogen/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Service caches the current setting in memory so the per-match hook
// (matching/handler.go) never needs a DB round trip on the hot path.
type Service struct {
	q       domain.Repository
	current atomic.Pointer[domain.State]
}

func NewService(ctx context.Context, q domain.Repository) (*Service, error) {
	row, err := q.GetAutoGenerateSetting(ctx)
	if err != nil {
		return nil, fmt.Errorf("autogen: load: %w", err)
	}
	s := &Service{q: q}
	s.current.Store(&domain.State{Enabled: row.Enabled, Threshold: int(row.Threshold)})
	return s, nil
}

func (s *Service) Get() domain.State {
	return *s.current.Load()
}

func (s *Service) Update(ctx context.Context, enabled bool, threshold int) (domain.State, error) {
	row, err := s.q.UpdateAutoGenerateSetting(ctx, sqlcgen.UpdateAutoGenerateSettingParams{
		Enabled:   enabled,
		Threshold: int32(threshold),
	})
	if err != nil {
		return domain.State{}, fmt.Errorf("autogen: update: %w", err)
	}
	st := domain.State{Enabled: row.Enabled, Threshold: int(row.Threshold)}
	s.current.Store(&st)
	return st, nil
}

// ShouldGenerate reports whether a job with this score should get an
// auto-enqueued resume, per the current cached setting.
func (s *Service) ShouldGenerate(score int) bool {
	st := s.Get()
	return st.Enabled && score >= st.Threshold
}
