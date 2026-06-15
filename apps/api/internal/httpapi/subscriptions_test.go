package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/subscriptions"
	"github.com/job-finder/api/internal/testutil"
)

type fakeSubProvider struct{}

func (f *fakeSubProvider) List(ctx context.Context) ([]dto.SubscriptionDto, error) {
	name := "Djinni Go"
	return []dto.SubscriptionDto{{ID: "sub1", Name: &name}}, nil
}

func (f *fakeSubProvider) ListBySource(ctx context.Context, sourceKey string) ([]dto.SubscriptionDto, error) {
	name := "Djinni Go"
	return []dto.SubscriptionDto{{ID: "sub1", Name: &name, SourceKey: sourceKey}}, nil
}

func (f *fakeSubProvider) Create(ctx context.Context, sourceKey, url string, name *string, enabled bool) (*dto.SubscriptionDto, error) {
	n := "New Sub"
	if name != nil {
		n = *name
	}
	return &dto.SubscriptionDto{ID: "sub-new", Name: &n, SourceKey: sourceKey}, nil
}

func (f *fakeSubProvider) Update(ctx context.Context, id string, in subscriptions.UpdateInput) (*dto.SubscriptionDto, error) {
	n := "Updated"
	if in.Name != nil {
		n = *in.Name
	}
	return &dto.SubscriptionDto{ID: id, Name: &n}, nil
}

func (f *fakeSubProvider) Delete(ctx context.Context, id string) error {
	return nil
}

type fakeSubRunner struct{}

func (f *fakeSubRunner) RunSubscription(ctx context.Context, subID string) error {
	return nil
}

func TestSubscriptionsList(t *testing.T) {
	h := &httpapi.SubscriptionsHandler{Subs: &fakeSubProvider{}, Ingestion: &fakeSubRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/subscriptions", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.SubscriptionDto
	testutil.ParseJSON(w, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(out))
	}
}

func TestSubscriptionsListBySource(t *testing.T) {
	h := &httpapi.SubscriptionsHandler{Subs: &fakeSubProvider{}, Ingestion: &fakeSubRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/subscriptions?source=djinni", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSubscriptionsCreate(t *testing.T) {
	h := &httpapi.SubscriptionsHandler{Subs: &fakeSubProvider{}, Ingestion: &fakeSubRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/subscriptions", map[string]any{
		"sourceKey": "djinni",
		"url":       "https://djinni.co/api/jobs?q=golang",
	}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSubscriptionsUpdate(t *testing.T) {
	name := "Updated"
	h := &httpapi.SubscriptionsHandler{Subs: &fakeSubProvider{}, Ingestion: &fakeSubRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PUT", "/api/subscriptions/sub1", map[string]any{"name": name}, map[string]string{"id": "sub1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSubscriptionsDelete(t *testing.T) {
	h := &httpapi.SubscriptionsHandler{Subs: &fakeSubProvider{}, Ingestion: &fakeSubRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "DELETE", "/api/subscriptions/sub1", nil, map[string]string{"id": "sub1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSubscriptionsRun(t *testing.T) {
	h := &httpapi.SubscriptionsHandler{Subs: &fakeSubProvider{}, Ingestion: &fakeSubRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/subscriptions/sub1/run", nil, map[string]string{"id": "sub1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
