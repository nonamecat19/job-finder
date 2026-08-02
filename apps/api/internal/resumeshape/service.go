// Package resumeshape holds the resume generation shape settings: the single
// persisted row describing how long a generated resume's summary is, how many
// bullets each experience entry keeps, whether the optional sections render,
// how many projects survive and how many pages the render loop targets.
//
// It mirrors internal/aifeature: settings CRUD with an in-memory cache, so the
// generation pipeline reads the config without a DB round trip. The shape
// *value type* lives in generation/domain, which this package imports —
// keeping the dependency one-way, with no import back from generation.
package resumeshape

import (
	"context"
	"fmt"
	"sync"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation/domain"
)

type Repository interface {
	GetResumeShapeSetting(ctx context.Context) (sqlcgen.ResumeShapeSetting, error)
	UpdateResumeShapeSetting(ctx context.Context, arg sqlcgen.UpdateResumeShapeSettingParams) (sqlcgen.ResumeShapeSetting, error)
}

// Service caches the config row in memory so a generation run resolves the
// shape for free. Update refreshes the cache, and only ever after the config
// has validated — a rejected update leaves both the row and the cache exactly
// as they were.
type Service struct {
	q Repository

	mu      sync.RWMutex
	current domain.ShapeConfig
}

// NewService loads the singleton row into the cache. A missing row falls back
// to the documented defaults rather than failing: the row is seeded by
// migration 00034, and generation must never be blocked by a settings read.
func NewService(ctx context.Context, q Repository) (*Service, error) {
	s := &Service{q: q, current: domain.DefaultShapeConfig()}
	row, err := q.GetResumeShapeSetting(ctx)
	if err != nil {
		return s, nil
	}
	s.current = rowToConfig(row)
	return s, nil
}

// Shape returns the cached config. It satisfies the generation service's
// ShapeProvider port structurally; the ctx argument is part of that port, not
// used here because the read is served from memory.
func (s *Service) Shape(context.Context) domain.ShapeConfig {
	return s.Get()
}

func (s *Service) Get() domain.ShapeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Update validates before it writes, so an invalid config is rejected whole:
// nothing is persisted and the cache still serves the previous values.
func (s *Service) Update(ctx context.Context, cfg domain.ShapeConfig) (domain.ShapeConfig, error) {
	if err := cfg.Validate(); err != nil {
		return domain.ShapeConfig{}, err
	}
	row, err := s.q.UpdateResumeShapeSetting(ctx, configToParams(cfg))
	if err != nil {
		return domain.ShapeConfig{}, fmt.Errorf("resumeshape: update: %w", err)
	}
	stored := rowToConfig(row)
	s.mu.Lock()
	s.current = stored
	s.mu.Unlock()
	return stored, nil
}

// Reset persists the documented defaults. Idempotent by construction.
func (s *Service) Reset(ctx context.Context) (domain.ShapeConfig, error) {
	return s.Update(ctx, domain.DefaultShapeConfig())
}

func rowToConfig(r sqlcgen.ResumeShapeSetting) domain.ShapeConfig {
	return domain.ShapeConfig{
		SummaryLines:         int(r.SummaryLines),
		SkillsEnabled:        r.SkillsEnabled,
		SkillsMaxGroups:      int(r.SkillsMaxGroups),
		ExperienceBulletsMin: int(r.ExperienceBulletsMin),
		ExperienceBulletsMax: int(r.ExperienceBulletsMax),
		TargetPages:          int(r.TargetPages),
		ProjectsEnabled:      r.ProjectsEnabled,
		ProjectsMin:          int(r.ProjectsMin),
		ProjectsMax:          int(r.ProjectsMax),
		ProjectBulletsMax:    int(r.ProjectBulletsMax),
	}
}

func configToParams(c domain.ShapeConfig) sqlcgen.UpdateResumeShapeSettingParams {
	return sqlcgen.UpdateResumeShapeSettingParams{
		SummaryLines:         int32(c.SummaryLines),
		SkillsEnabled:        c.SkillsEnabled,
		SkillsMaxGroups:      int32(c.SkillsMaxGroups),
		ExperienceBulletsMin: int32(c.ExperienceBulletsMin),
		ExperienceBulletsMax: int32(c.ExperienceBulletsMax),
		TargetPages:          int32(c.TargetPages),
		ProjectsEnabled:      c.ProjectsEnabled,
		ProjectsMin:          int32(c.ProjectsMin),
		ProjectsMax:          int32(c.ProjectsMax),
		ProjectBulletsMax:    int32(c.ProjectBulletsMax),
	}
}
