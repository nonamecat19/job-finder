package profile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/profile"
)

type fakeRepo struct {
	profile.Repository
	rows    []sqlcgen.Profile
	listErr error
}

func (f *fakeRepo) ListProfiles(ctx context.Context) ([]sqlcgen.Profile, error) {
	return f.rows, f.listErr
}

func TestListEmpty(t *testing.T) {
	svc := profile.NewService(&fakeRepo{}, nil)
	out, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

func TestListError(t *testing.T) {
	svc := profile.NewService(&fakeRepo{listErr: errors.New("db down")}, nil)
	if _, err := svc.List(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}
