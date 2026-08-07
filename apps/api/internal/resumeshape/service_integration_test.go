//go:build integration

package resumeshape_test

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/resumeshape"
)

func TestResumeShapeSettingPersistsAcrossServiceRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	svc, err := resumeshape.NewService(ctx, testDB.Queries)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if got := svc.Get(); got != domain.DefaultShapeConfig() {
		t.Fatalf("initial config = %+v, want the defaults", got)
	}

	want := domain.ShapeConfig{
		SummaryLines: 6, SkillsEnabled: false, SkillsMaxGroups: 4,
		ExperienceBulletsMin: 3, ExperienceBulletsMax: 7, TargetPages: 1,
		ProjectsEnabled: true, ProjectsMin: 1, ProjectsMax: 3, ProjectBulletsMax: 2,
		CertificationsEnabled: false, CertificationsMin: 0, CertificationsMax: 5,
		FontSize: 9,
	}
	if _, err := svc.Update(ctx, want); err != nil {
		t.Fatalf("Update: %v", err)
	}

	restarted, err := resumeshape.NewService(ctx, testDB.Queries)
	if err != nil {
		t.Fatalf("NewService after restart: %v", err)
	}
	if got := restarted.Get(); got != want {
		t.Errorf("config after restart = %+v, want %+v", got, want)
	}

	if _, err := restarted.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	afterReset, err := resumeshape.NewService(ctx, testDB.Queries)
	if err != nil {
		t.Fatalf("NewService after reset: %v", err)
	}
	if got := afterReset.Get(); got != domain.DefaultShapeConfig() {
		t.Errorf("config after reset+restart = %+v, want the defaults", got)
	}
}

func TestResumeShapeSettingRejectsOutOfRangeAtTheDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	_, err := testDB.Pool.Exec(ctx, `UPDATE "ResumeShapeSetting" SET "targetPages" = 9 WHERE "id" = 'default'`)
	if err == nil {
		t.Fatal("out-of-range targetPages was accepted, want the CHECK constraint to reject it")
	}
}
