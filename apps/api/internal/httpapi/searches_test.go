package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/testutil"
)

type fakeSearchProvider struct{}

func (f *fakeSearchProvider) ListSearches(ctx context.Context) ([]dto.SavedSearchDto, error) {
	return []dto.SavedSearchDto{{ID: "s1", Name: "Remote Go"}}, nil
}

func (f *fakeSearchProvider) CreateSearch(ctx context.Context, name string, query dto.SearchQuery, cron string, enabled bool) (*dto.SavedSearchDto, error) {
	return &dto.SavedSearchDto{ID: "s-new", Name: name}, nil
}

func (f *fakeSearchProvider) UpdateSearch(ctx context.Context, id string, in ingestion.UpdateSearchInput) (*dto.SavedSearchDto, error) {
	return &dto.SavedSearchDto{ID: id, Name: *in.Name}, nil
}

func (f *fakeSearchProvider) DeleteSearch(ctx context.Context, id string) error {
	return nil
}

func (f *fakeSearchProvider) RunSearch(ctx context.Context, searchID string) ([]string, error) {
	return []string{"djinni"}, nil
}

func (f *fakeSearchProvider) RecentRuns(ctx context.Context, limit int32) ([]dto.SourceRunDto, error) {
	return []dto.SourceRunDto{}, nil
}

func TestSearchesList(t *testing.T) {
	h := &httpapi.SearchesHandler{Ingestion: &fakeSearchProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/searches", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.SavedSearchDto
	testutil.ParseJSON(w, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 search, got %d", len(out))
	}
}

func TestSearchesCreate(t *testing.T) {
	h := &httpapi.SearchesHandler{Ingestion: &fakeSearchProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/searches", map[string]any{
		"name":  "Test Search",
		"query": map[string]any{"text": "golang"},
		"cron":  "0 8 * * *",
	}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSearchesUpdate(t *testing.T) {
	name := "Updated"
	h := &httpapi.SearchesHandler{Ingestion: &fakeSearchProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PUT", "/api/searches/s1", map[string]any{"name": name}, map[string]string{"id": "s1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSearchesDelete(t *testing.T) {
	h := &httpapi.SearchesHandler{Ingestion: &fakeSearchProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "DELETE", "/api/searches/s1", nil, map[string]string{"id": "s1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSearchesRun(t *testing.T) {
	h := &httpapi.SearchesHandler{Ingestion: &fakeSearchProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/searches/s1/run", nil, map[string]string{"id": "s1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSearchesRecentRuns(t *testing.T) {
	h := &httpapi.SearchesHandler{Ingestion: &fakeSearchProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/searches/runs/recent", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
