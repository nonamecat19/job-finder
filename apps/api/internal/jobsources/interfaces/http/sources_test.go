package http_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	jobsourceshttp "github.com/job-finder/api/internal/jobsources/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeSourcesProvider struct {
	sources []dto.JobSourceDto
	err     error
}

func (f *fakeSourcesProvider) List(ctx context.Context) ([]dto.JobSourceDto, error) {
	return f.sources, f.err
}

func (f *fakeSourcesProvider) Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error) {
	return &dto.JobSourceDto{Key: key, Enabled: enabled != nil && *enabled}, f.err
}

func (f *fakeSourcesProvider) Test(ctx context.Context, key string) (bool, string) {
	return true, ""
}

type fakeEnrichmentRunner struct{}

func (f *fakeEnrichmentRunner) EnqueueBackfill(ctx context.Context, sourceKey string, limit int32) (int, error) {
	return int(limit), nil
}

type fakeSourceRunner struct{}

func (f *fakeSourceRunner) RunSource(ctx context.Context, sourceKey string) error {
	return nil
}

func TestSourcesList(t *testing.T) {
	fake := &fakeSourcesProvider{
		sources: []dto.JobSourceDto{
			{Key: "djinni", Enabled: true},
			{Key: "dou", Enabled: false},
		},
	}
	h := &jobsourceshttp.SourcesHandler{Sources: fake, Enrichment: &fakeEnrichmentRunner{}, Ingestion: &fakeSourceRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/sources", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.JobSourceDto
	testutil.ParseJSON(w, &out)
	if len(out) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(out))
	}
	if out[0].Key != "djinni" {
		t.Fatalf("expected djinni, got %s", out[0].Key)
	}
}

func TestSourcesUpdate(t *testing.T) {
	fake := &fakeSourcesProvider{}
	h := &jobsourceshttp.SourcesHandler{Sources: fake, Enrichment: &fakeEnrichmentRunner{}, Ingestion: &fakeSourceRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "PUT", "/api/sources/djinni", map[string]any{"enabled": true}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSourcesTest(t *testing.T) {
	h := &jobsourceshttp.SourcesHandler{Sources: &fakeSourcesProvider{}, Enrichment: &fakeEnrichmentRunner{}, Ingestion: &fakeSourceRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/sources/djinni/test", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSourcesEnqueueBackfill(t *testing.T) {
	h := &jobsourceshttp.SourcesHandler{Sources: &fakeSourcesProvider{}, Enrichment: &fakeEnrichmentRunner{}, Ingestion: &fakeSourceRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/sources/djinni/enrich", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSourcesRun(t *testing.T) {
	h := &jobsourceshttp.SourcesHandler{Sources: &fakeSourcesProvider{}, Enrichment: &fakeEnrichmentRunner{}, Ingestion: &fakeSourceRunner{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/sources/djinni/run", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
