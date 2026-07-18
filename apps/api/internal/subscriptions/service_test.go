package subscriptions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/subscriptions"
)

type fakeRepo struct {
	subscriptions.Repository
	rows    []sqlcgen.Subscription
	listErr error
}

func (f *fakeRepo) ListSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error) {
	return f.rows, f.listErr
}

type fakeSources struct{ subscriptions.SourceEnsurer }

func TestListEmpty(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

func TestListError(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{listErr: errors.New("db down")}, &fakeSources{})
	if _, err := svc.List(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}
