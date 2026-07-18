package applications_test

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/applications"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
)

// fakeRepo embeds the Repository port; the test overrides only what it needs.
type fakeRepo struct {
	applications.Repository
	rows    []sqlcgen.ListApplicationsRow
	listErr error
}

func (f *fakeRepo) ListApplications(ctx context.Context, status *string) ([]sqlcgen.ListApplicationsRow, error) {
	return f.rows, f.listErr
}

func TestListEmpty(t *testing.T) {
	svc := applications.NewService(&fakeRepo{})
	out, err := svc.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
	// non-nil empty slice (JSON serializes as [], not null)
	if out == nil {
		t.Error("expected non-nil empty slice")
	}
	var _ []dto.ApplicationDto = out
}

func TestListError(t *testing.T) {
	svc := applications.NewService(&fakeRepo{listErr: errors.New("db down")})
	if _, err := svc.List(context.Background(), nil); err == nil {
		t.Fatal("expected error, got nil")
	}
}
