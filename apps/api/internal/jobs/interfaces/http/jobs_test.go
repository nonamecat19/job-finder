package http_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobs"
	jobshttp "github.com/job-finder/api/internal/jobs/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeJobsProvider struct {
	jobs []dto.JobDto
}

func (f *fakeJobsProvider) List(ctx context.Context, params jobs.ListParams) (dto.JobListResponse, error) {
	return dto.JobListResponse{Items: f.jobs, Total: int64(len(f.jobs))}, nil
}

func (f *fakeJobsProvider) Get(ctx context.Context, id string) (dto.JobDto, error) {
	return dto.JobDto{ID: id, Title: "Test Job"}, nil
}

func (f *fakeJobsProvider) Shortlist(ctx context.Context, id string) (dto.JobDto, error) {
	return dto.JobDto{ID: id, Status: "shortlisted"}, nil
}

func (f *fakeJobsProvider) Hide(ctx context.Context, id string) (dto.JobDto, error) {
	return dto.JobDto{ID: id, Status: "hidden"}, nil
}

func (f *fakeJobsProvider) Unhide(ctx context.Context, id string) (dto.JobDto, error) {
	return dto.JobDto{ID: id, Status: "found"}, nil
}

func (f *fakeJobsProvider) DeleteAll(ctx context.Context) (int64, error) {
	n := int64(len(f.jobs))
	f.jobs = nil
	return n, nil
}

func (f *fakeJobsProvider) EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error) {
	return map[string]any{"taskId": "task-123"}, nil
}

type fakeDocLister struct{}

func (f *fakeDocLister) ListDocuments(ctx context.Context, jobID string) ([]dto.GeneratedDocumentDto, error) {
	return []dto.GeneratedDocumentDto{}, nil
}

func (f *fakeDocLister) ListDocumentStatuses(ctx context.Context, jobID string) ([]dto.DocumentStatusDto, error) {
	return []dto.DocumentStatusDto{}, nil
}

func TestJobsList(t *testing.T) {
	fake := &fakeJobsProvider{jobs: []dto.JobDto{{ID: "1", Title: "Engineer"}}}
	h := &jobshttp.JobsHandler{Jobs: fake, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestJobsDeleteAll(t *testing.T) {
	fake := &fakeJobsProvider{jobs: []dto.JobDto{{ID: "1"}, {ID: "2"}}}
	h := &jobshttp.JobsHandler{Jobs: fake, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "DELETE", "/api/jobs", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]int64
	testutil.ParseJSON(w, &out)
	if out["deleted"] != 2 {
		t.Fatalf("expected deleted=2, got %d", out["deleted"])
	}
}

func TestJobsGet(t *testing.T) {
	h := &jobshttp.JobsHandler{Jobs: &fakeJobsProvider{}, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out dto.JobDto
	testutil.ParseJSON(w, &out)
	if out.ID != "job-1" {
		t.Fatalf("expected job-1, got %s", out.ID)
	}
}

func TestJobsShortlist(t *testing.T) {
	h := &jobshttp.JobsHandler{Jobs: &fakeJobsProvider{}, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/shortlist", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestJobsHide(t *testing.T) {
	h := &jobshttp.JobsHandler{Jobs: &fakeJobsProvider{}, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/hide", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestJobsGenerate(t *testing.T) {
	h := &jobshttp.JobsHandler{Jobs: &fakeJobsProvider{}, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/generate", map[string]any{"type": "rendercv"}, map[string]string{"id": "job-1"})
	if w.Code != 202 {
		t.Fatalf("expected 202, got %d", w.Code)
	}
}

func TestJobsDocuments(t *testing.T) {
	h := &jobshttp.JobsHandler{Jobs: &fakeJobsProvider{}, Generation: &fakeDocLister{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/documents", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
