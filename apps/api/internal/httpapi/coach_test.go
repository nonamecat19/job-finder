package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/coach/application"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakeCoachProvider struct {
	resp      dto.FitGapAssessmentDto
	assessErr error
	cachedErr error
}

func (f *fakeCoachProvider) Assess(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error) {
	if f.assessErr != nil {
		return dto.FitGapAssessmentDto{}, f.assessErr
	}
	f.resp.JobID = jobID
	return f.resp, nil
}

func (f *fakeCoachProvider) CachedAssessment(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error) {
	if f.cachedErr != nil {
		return dto.FitGapAssessmentDto{}, f.cachedErr
	}
	f.resp.JobID = jobID
	return f.resp, nil
}

func sampleAssessment() dto.FitGapAssessmentDto {
	return dto.FitGapAssessmentDto{
		TotalMustHaves:  2,
		FailedMustHaves: 1,
		CoveragePct:     50.0,
		Gaps: []dto.FitGapItemDto{
			{
				Term:     "Kubernetes",
				Polarity: "required",
				AdjacentEvidence: []dto.FitGapEvidenceDto{
					{SourceEntry: "DevOps Engineer, Acme (2022-2024)", SourceBullet: "Ran Docker Swarm clusters", Proximity: "close", Rephrase: "Ran container orchestration adjacent to Kubernetes"},
				},
			},
		},
	}
}

func TestCoachAssessPost(t *testing.T) {
	h := &httpapi.CoachHandler{Coach: &fakeCoachProvider{resp: sampleAssessment()}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/jobs/job-1/coach/assess", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out dto.FitGapAssessmentDto
	testutil.ParseJSON(w, &out)

	if out.JobID != "job-1" {
		t.Errorf("expected jobId echoed, got %q", out.JobID)
	}
	if out.TotalMustHaves != 2 || out.FailedMustHaves != 1 {
		t.Errorf("unexpected counts: %+v", out)
	}
	if len(out.Gaps) != 1 || out.Gaps[0].Term != "Kubernetes" {
		t.Fatalf("unexpected gaps: %+v", out.Gaps)
	}
	if len(out.Gaps[0].AdjacentEvidence) != 1 || out.Gaps[0].AdjacentEvidence[0].Rephrase == "" {
		t.Errorf("expected adjacent evidence with a rephrase, got %+v", out.Gaps[0])
	}
}

func TestCoachAssessPost_NoDiff(t *testing.T) {
	h := &httpapi.CoachHandler{Coach: &fakeCoachProvider{assessErr: application.ErrNoDiff}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/jobs/job-1/coach/assess", nil, map[string]string{"id": "job-1"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCoachAssessmentGet(t *testing.T) {
	h := &httpapi.CoachHandler{Coach: &fakeCoachProvider{resp: sampleAssessment()}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/coach/assessment", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out dto.FitGapAssessmentDto
	testutil.ParseJSON(w, &out)
	if out.JobID != "job-1" {
		t.Errorf("expected jobId echoed, got %q", out.JobID)
	}
}

func TestCoachAssessmentGet_NotAssessed(t *testing.T) {
	h := &httpapi.CoachHandler{Coach: &fakeCoachProvider{cachedErr: application.ErrNotAssessed}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/coach/assessment", nil, map[string]string{"id": "job-1"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
