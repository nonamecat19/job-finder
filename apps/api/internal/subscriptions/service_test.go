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

func (f *fakeRepo) CreateSubscription(ctx context.Context, arg sqlcgen.CreateSubscriptionParams) (sqlcgen.Subscription, error) {
	return sqlcgen.Subscription{SourceKey: arg.SourceKey, Name: arg.Name, Url: arg.Url, Enabled: arg.Enabled}, nil
}

type fakeSources struct{ subscriptions.SourceEnsurer }

func (f *fakeSources) GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error) {
	return sqlcgen.JobSource{Key: key}, nil
}

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

func TestCreateIndeedSubscription_ValidURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "indeed", "https://www.indeed.com/jobs?q=golang&l=remote", nil, true)
	if err != nil {
		t.Fatalf("expected valid indeed search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "indeed" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateIndeedSubscription_RejectsNonIndeedURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "indeed", "https://example.com/not-indeed", nil, true); err == nil {
		t.Fatal("expected non-indeed url to be rejected")
	}
}

func TestCreateIndeedSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "indeed", "https://www.indeed.com/viewjob?jk=abc123", nil, true); err == nil {
		t.Fatal("expected single job-posting url to be rejected, want a search results url")
	}
}

func TestCreateDouSubscription_UnaffectedByIndeedValidation(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "dou", "https://jobs.dou.ua/vacancies/?category=Golang", nil, true); err != nil {
		t.Fatalf("expected non-indeed source to be unaffected by indeed-specific validation, got: %v", err)
	}
}
