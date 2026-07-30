package httpapi_test

import (
	"context"
	"testing"

	applications "github.com/job-finder/api/internal/applications/application"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakeAppProvider struct{}

func (f *fakeAppProvider) List(ctx context.Context, status *string) ([]dto.ApplicationDto, error) {
	return []dto.ApplicationDto{{ID: "a1", Status: "applied"}}, nil
}

func (f *fakeAppProvider) Update(ctx context.Context, id string, in applications.UpdateInput) (dto.ApplicationDto, error) {
	return dto.ApplicationDto{ID: id, Status: "interview"}, nil
}

func (f *fakeAppProvider) Stats(ctx context.Context) (dto.StatsDto, error) {
	return dto.StatsDto{JobsTotal: 10}, nil
}

func TestApplicationsList(t *testing.T) {
	h := &httpapi.ApplicationsHandler{Applications: &fakeAppProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/applications", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.ApplicationDto
	testutil.ParseJSON(w, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 application, got %d", len(out))
	}
}

func TestApplicationsUpdate(t *testing.T) {
	h := &httpapi.ApplicationsHandler{Applications: &fakeAppProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PATCH", "/api/applications/a1", map[string]any{"status": "interview"}, map[string]string{"id": "a1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestApplicationsStats(t *testing.T) {
	h := &httpapi.ApplicationsHandler{Applications: &fakeAppProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/stats", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
