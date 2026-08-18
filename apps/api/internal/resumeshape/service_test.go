package resumeshape

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation/domain"
)

type fakeRepo struct {
	row     sqlcgen.ResumeShapeSetting
	getErr  error
	updErr  error
	gets    int
	updates int
}

func (f *fakeRepo) GetResumeShapeSetting(context.Context) (sqlcgen.ResumeShapeSetting, error) {
	f.gets++
	if f.getErr != nil {
		return sqlcgen.ResumeShapeSetting{}, f.getErr
	}
	return f.row, nil
}

func (f *fakeRepo) UpdateResumeShapeSetting(_ context.Context, arg sqlcgen.UpdateResumeShapeSettingParams) (sqlcgen.ResumeShapeSetting, error) {
	f.updates++
	if f.updErr != nil {
		return sqlcgen.ResumeShapeSetting{}, f.updErr
	}
	f.row = sqlcgen.ResumeShapeSetting{
		ID:                    "default",
		SummaryLines:          arg.SummaryLines,
		SummaryEnabled:        arg.SummaryEnabled,
		SkillsEnabled:         arg.SkillsEnabled,
		SkillsMaxGroups:       arg.SkillsMaxGroups,
		ExperienceEnabled:     arg.ExperienceEnabled,
		ExperienceBulletsMin:  arg.ExperienceBulletsMin,
		ExperienceBulletsMax:  arg.ExperienceBulletsMax,
		TargetPages:           arg.TargetPages,
		ProjectsEnabled:       arg.ProjectsEnabled,
		ProjectsMin:           arg.ProjectsMin,
		ProjectsMax:           arg.ProjectsMax,
		ProjectBulletsMax:     arg.ProjectBulletsMax,
		CertificationsEnabled: arg.CertificationsEnabled,
		CertificationsMin:     arg.CertificationsMin,
		CertificationsMax:     arg.CertificationsMax,
		EducationEnabled:      arg.EducationEnabled,
		FontSize:              arg.FontSize,
	}
	return f.row, nil
}

func defaultRow() sqlcgen.ResumeShapeSetting {
	return sqlcgen.ResumeShapeSetting{
		ID: "default", SummaryLines: 4, SummaryEnabled: true, SkillsEnabled: true, SkillsMaxGroups: 0,
		ExperienceEnabled: true, ExperienceBulletsMin: 8, ExperienceBulletsMax: 10, TargetPages: 2,
		ProjectsEnabled: true, ProjectsMin: 0, ProjectsMax: 0, ProjectBulletsMax: 0,
		CertificationsEnabled: true, CertificationsMin: 0, CertificationsMax: 0,
		EducationEnabled: true,
		FontSize:         10,
	}
}

func newTestService(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	s, err := NewService(context.Background(), repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return s
}

func TestNewServiceLoadsRow(t *testing.T) {
	repo := &fakeRepo{row: defaultRow()}
	repo.row.TargetPages = 3
	s := newTestService(t, repo)

	if got := s.Get().TargetPages; got != 3 {
		t.Errorf("TargetPages = %d, want 3 (loaded from the row)", got)
	}
}

func TestNewServiceFallsBackToDefaultsWhenRowMissing(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("no rows")}
	s := newTestService(t, repo)

	if got := s.Get(); got != domain.DefaultShapeConfig() {
		t.Errorf("Get() = %+v, want the defaults", got)
	}
}

func TestGetServesFromCache(t *testing.T) {
	repo := &fakeRepo{row: defaultRow()}
	s := newTestService(t, repo)

	for i := 0; i < 5; i++ {
		s.Get()
		s.Shape(context.Background())
	}
	if repo.gets != 1 {
		t.Errorf("repo reads = %d, want 1 (load at construction only)", repo.gets)
	}
}

func TestUpdateRejectsInvalidConfigWithoutWriting(t *testing.T) {
	repo := &fakeRepo{row: defaultRow()}
	s := newTestService(t, repo)

	bad := domain.DefaultShapeConfig()
	bad.TargetPages = 4
	if _, err := s.Update(context.Background(), bad); err == nil {
		t.Fatal("Update with targetPages 4 = nil error, want a validation error")
	}
	if repo.updates != 0 {
		t.Errorf("repo writes = %d, want 0 (validation precedes any write)", repo.updates)
	}
	if got := s.Get(); got != domain.DefaultShapeConfig() {
		t.Errorf("cached config = %+v, want it untouched at the defaults", got)
	}
}

func TestUpdateRefreshesCache(t *testing.T) {
	repo := &fakeRepo{row: defaultRow()}
	s := newTestService(t, repo)

	want := domain.ShapeConfig{
		SummaryLines: 2, SummaryEnabled: true, SkillsEnabled: false, SkillsMaxGroups: 3,
		ExperienceEnabled: true, ExperienceBulletsMin: 4, ExperienceBulletsMax: 5, TargetPages: 1,
		ProjectsEnabled: true, ProjectsMin: 1, ProjectsMax: 2, ProjectBulletsMax: 3,
		CertificationsEnabled: true, CertificationsMin: 1, CertificationsMax: 5,
		EducationEnabled: true,
		FontSize:         12,
	}
	got, err := s.Update(context.Background(), want)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got != want {
		t.Errorf("Update returned %+v, want %+v", got, want)
	}
	if cached := s.Get(); cached != want {
		t.Errorf("cached config = %+v, want %+v", cached, want)
	}
}

func TestUpdateSurfacesPersistenceFailure(t *testing.T) {
	repo := &fakeRepo{row: defaultRow(), updErr: errors.New("db down")}
	s := newTestService(t, repo)

	if _, err := s.Update(context.Background(), domain.DefaultShapeConfig()); err == nil {
		t.Fatal("Update = nil error, want the persistence failure surfaced")
	}
	if got := s.Get(); got != domain.DefaultShapeConfig() {
		t.Errorf("cached config = %+v, want it untouched", got)
	}
}

func TestResetRestoresDefaults(t *testing.T) {
	repo := &fakeRepo{row: defaultRow()}
	s := newTestService(t, repo)

	changed := domain.DefaultShapeConfig()
	changed.TargetPages = 1
	changed.SkillsEnabled = false
	changed.CertificationsEnabled = false
	changed.CertificationsMax = 3
	if _, err := s.Update(context.Background(), changed); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got != domain.DefaultShapeConfig() {
		t.Errorf("Reset returned %+v, want the defaults", got)
	}
	if !got.CertificationsEnabled || got.CertificationsMin != 0 || got.CertificationsMax != 0 {
		t.Errorf("Reset certifications fields = enabled=%v min=%d max=%d, want true/0/0",
			got.CertificationsEnabled, got.CertificationsMin, got.CertificationsMax)
	}
	if cached := s.Get(); cached != domain.DefaultShapeConfig() {
		t.Errorf("cached config = %+v, want the defaults", cached)
	}

	again, err := s.Reset(context.Background())
	if err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	if again != domain.DefaultShapeConfig() {
		t.Errorf("second Reset returned %+v, want the defaults", again)
	}
}
