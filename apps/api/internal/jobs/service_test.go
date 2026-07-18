package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/jobs"
)

// fakeRepo embeds the Repository port so unimplemented methods satisfy the
// interface; the test overrides only what it exercises. This is the payoff of
// the hexagonal seam: the use-case is now testable without a real database.
type fakeRepo struct {
	jobs.Repository
	deleted    int64
	deleteErr  error
	deleteCall int
}

func (f *fakeRepo) DeleteAllJobs(ctx context.Context) (int64, error) {
	f.deleteCall++
	return f.deleted, f.deleteErr
}

type fakeEnqueuer struct{ jobs.Enqueuer }

func TestServiceDeleteAll(t *testing.T) {
	repo := &fakeRepo{deleted: 7}
	svc := jobs.NewService(repo, &fakeEnqueuer{})

	n, err := svc.DeleteAll(context.Background())
	if err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if n != 7 {
		t.Errorf("deleted = %d, want 7", n)
	}
	if repo.deleteCall != 1 {
		t.Errorf("DeleteAllJobs called %d times, want 1", repo.deleteCall)
	}
}

func TestServiceDeleteAllError(t *testing.T) {
	repo := &fakeRepo{deleteErr: errors.New("boom")}
	svc := jobs.NewService(repo, &fakeEnqueuer{})

	if _, err := svc.DeleteAll(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}
