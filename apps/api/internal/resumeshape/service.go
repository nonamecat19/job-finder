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

type Service struct {
	q Repository

	mu      sync.RWMutex
	current domain.ShapeConfig
}

func NewService(ctx context.Context, q Repository) (*Service, error) {
	s := &Service{q: q, current: domain.DefaultShapeConfig()}
	row, err := q.GetResumeShapeSetting(ctx)
	if err != nil {
		return s, nil
	}
	s.current = rowToConfig(row)
	return s, nil
}

func (s *Service) Shape(context.Context) domain.ShapeConfig {
	return s.Get()
}

func (s *Service) Get() domain.ShapeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

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

func (s *Service) Reset(ctx context.Context) (domain.ShapeConfig, error) {
	return s.Update(ctx, domain.DefaultShapeConfig())
}

func rowToConfig(r sqlcgen.ResumeShapeSetting) domain.ShapeConfig {
	return domain.ShapeConfig{
		SummaryLines:          int(r.SummaryLines),
		SummaryEnabled:        r.SummaryEnabled,
		SkillsEnabled:         r.SkillsEnabled,
		SkillsMaxGroups:       int(r.SkillsMaxGroups),
		ExperienceEnabled:     r.ExperienceEnabled,
		ExperienceBulletsMin:  int(r.ExperienceBulletsMin),
		ExperienceBulletsMax:  int(r.ExperienceBulletsMax),
		TargetPages:           int(r.TargetPages),
		ProjectsEnabled:       r.ProjectsEnabled,
		ProjectsMin:           int(r.ProjectsMin),
		ProjectsMax:           int(r.ProjectsMax),
		ProjectBulletsMax:     int(r.ProjectBulletsMax),
		CertificationsEnabled: r.CertificationsEnabled,
		CertificationsMin:     int(r.CertificationsMin),
		CertificationsMax:     int(r.CertificationsMax),
		EducationEnabled:      r.EducationEnabled,
		FontSize:              int(r.FontSize),
	}
}

func configToParams(c domain.ShapeConfig) sqlcgen.UpdateResumeShapeSettingParams {
	return sqlcgen.UpdateResumeShapeSettingParams{
		SummaryLines:          int32(c.SummaryLines),
		SummaryEnabled:        c.SummaryEnabled,
		SkillsEnabled:         c.SkillsEnabled,
		SkillsMaxGroups:       int32(c.SkillsMaxGroups),
		ExperienceEnabled:     c.ExperienceEnabled,
		ExperienceBulletsMin:  int32(c.ExperienceBulletsMin),
		ExperienceBulletsMax:  int32(c.ExperienceBulletsMax),
		TargetPages:           int32(c.TargetPages),
		ProjectsEnabled:       c.ProjectsEnabled,
		ProjectsMin:           int32(c.ProjectsMin),
		ProjectsMax:           int32(c.ProjectsMax),
		ProjectBulletsMax:     int32(c.ProjectBulletsMax),
		CertificationsEnabled: c.CertificationsEnabled,
		CertificationsMin:     int32(c.CertificationsMin),
		CertificationsMax:     int32(c.CertificationsMax),
		EducationEnabled:      c.EducationEnabled,
		FontSize:              int32(c.FontSize),
	}
}
