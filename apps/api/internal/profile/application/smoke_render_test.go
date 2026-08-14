package application

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/profile/domain"
)

type smokeFakeRepo struct {
	domain.Repository
	defaultProfile sqlcgen.Profile
	defaultErr     error
	getProfile     sqlcgen.Profile
}

func (f *smokeFakeRepo) GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error) {
	return f.defaultProfile, f.defaultErr
}

func (f *smokeFakeRepo) UpdateProfile(ctx context.Context, params sqlcgen.UpdateProfileParams) error {
	return nil
}

func (f *smokeFakeRepo) GetProfile(ctx context.Context, id pgtype.UUID) (sqlcgen.Profile, error) {
	return f.getProfile, nil
}

func (f *smokeFakeRepo) UpdateProfileEmbedding(ctx context.Context, params sqlcgen.UpdateProfileEmbeddingParams) error {
	return nil
}

func (f *smokeFakeRepo) CreateProfile(ctx context.Context, params sqlcgen.CreateProfileParams) (sqlcgen.Profile, error) {
	return f.getProfile, nil
}

type stubEmbedder struct{ llm.Provider }

func (stubEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0}, nil
}

func assertNoSmokeDirs(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "rendercv-smoke-") {
			t.Errorf("leaked temp dir: %s", e.Name())
		}
	}
}

func TestSaveConfig_ValidYaml(t *testing.T) {
	t.Cleanup(func() { assertNoSmokeDirs(t) })

	uid, _ := dbutil.ParseUUID("00000000-0000-0000-0000-000000000001")
	repo := &smokeFakeRepo{
		defaultProfile: sqlcgen.Profile{ID: uid, Name: "Default"},
		getProfile:     sqlcgen.Profile{ID: uid, Name: "Test User"},
	}
	svc := NewService(repo, stubEmbedder{})

	yaml := "cv:\n  name: Test User\n"
	result, err := svc.SaveConfig(context.Background(), yaml)
	if err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if result.Name != "Test User" {
		t.Errorf("name = %q, want %q", result.Name, "Test User")
	}
}

func TestSaveConfig_MissingCvBlock(t *testing.T) {
	t.Cleanup(func() { assertNoSmokeDirs(t) })

	repo := &smokeFakeRepo{}
	svc := NewService(repo, stubEmbedder{})

	yaml := "design:\n  theme: classic\n"
	_, err := svc.SaveConfig(context.Background(), yaml)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cv") {
		t.Errorf("error should mention 'cv', got: %v", err)
	}
}
