package http_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/manualadd/application"
	"github.com/job-finder/api/internal/manualadd/domain"
	manualaddhttp "github.com/job-finder/api/internal/manualadd/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeAddService struct {
	result   domain.Result
	err      error
	seen     string
	fillIn   application.FillIn
	fillInOk bool
}

func (f *fakeAddService) Add(_ context.Context, rawURL string) (domain.Result, error) {
	f.seen = rawURL
	return f.result, f.err
}

func (f *fakeAddService) SaveFillIn(_ context.Context, in application.FillIn) (domain.Result, error) {
	f.fillIn, f.fillInOk = in, true
	return f.result, f.err
}

func router(svc *fakeAddService) *chi.Mux {
	h := &manualaddhttp.ManualAddHandler{Manual: svc}
	return testutil.SetupRouter(h.Mount)
}

func TestManualAdd_CreatedIs201WithTheJob(t *testing.T) {
	svc := &fakeAddService{result: domain.Result{
		Outcome: domain.OutcomeCreated,
		Job:     &dto.JobDto{ID: "job-1", SourceKey: "djinni", Title: "Senior Go Engineer"},
	}}

	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual",
		map[string]any{"url": "https://djinni.co/jobs/123456-senior-go-engineer/"}, nil)

	if w.Code != 201 {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	var out dto.ManualAddResultDto
	testutil.ParseJSON(w, &out)
	if out.Outcome != "created" {
		t.Errorf("outcome = %q", out.Outcome)
	}
	if out.Job == nil || out.Job.ID != "job-1" {
		t.Errorf("expected the full JobDto the feed already consumes, got %+v", out.Job)
	}
	if out.Draft != nil || out.Kind != nil {
		t.Errorf("expected no draft or kind on a created result, got %+v", out)
	}
}

func TestManualAdd_DuplicateIs200WithTheExistingJob(t *testing.T) {
	svc := &fakeAddService{result: domain.Result{
		Outcome: domain.OutcomeDuplicate,
		Job:     &dto.JobDto{ID: "job-existing"},
		Reason:  "This vacancy is already in your feed.",
	}}

	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual",
		map[string]any{"url": "https://djinni.co/jobs/1-a/"}, nil)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 — a duplicate is not an error", w.Code)
	}
	var out dto.ManualAddResultDto
	testutil.ParseJSON(w, &out)
	if out.Outcome != "duplicate" || out.Job == nil || out.Job.ID != "job-existing" {
		t.Errorf("expected the existing vacancy so the client can navigate to it, got %+v", out)
	}
	if out.Reason == nil {
		t.Error("expected an operator-facing reason")
	}
}

func TestManualAdd_FailureStatusPerKind(t *testing.T) {
	tests := []struct {
		kind domain.FailureKind
		want int
	}{
		{domain.KindInvalidURL, 400},
		{domain.KindNotAPosting, 422},
		{domain.KindNoReader, 422},
		{domain.KindUnreachable, 422},
		{domain.KindBlocked, 422},
		{domain.KindTimedOut, 422},
		{domain.KindIncomplete, 422},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			svc := &fakeAddService{result: domain.Result{
				Outcome: domain.OutcomeFailed, Kind: tt.kind, Reason: "because",
			}}

			w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual",
				map[string]any{"url": "https://example.com/x"}, nil)

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d", w.Code, tt.want)
			}
			var out dto.ManualAddResultDto
			testutil.ParseJSON(w, &out)
			if out.Outcome != "failed" {
				t.Errorf("outcome = %q", out.Outcome)
			}
			if out.Kind == nil || *out.Kind != string(tt.kind) {
				t.Errorf("expected the client to switch on kind, got %+v", out.Kind)
			}
			if out.Job != nil {
				t.Error("a failed attempt must carry no vacancy")
			}
		})
	}
}

func TestManualAdd_NeedsFillInIs200WithTheDraft(t *testing.T) {
	title := "Senior Go Engineer"
	svc := &fakeAddService{result: domain.Result{
		Outcome: domain.OutcomeNeedsFillIn,
		Kind:    domain.KindIncomplete,
		Reason:  "The page was read but is missing description.",
		Draft:   &domain.Draft{URL: "https://djinni.co/jobs/1-a/", Title: &title, Remote: true},
	}}

	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual",
		map[string]any{"url": "https://djinni.co/jobs/1-a/"}, nil)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 — a recoverable state is not a failed request", w.Code)
	}
	var out dto.ManualAddResultDto
	testutil.ParseJSON(w, &out)
	if out.Outcome != "needs_fill_in" {
		t.Errorf("outcome = %q", out.Outcome)
	}
	if out.Draft == nil || out.Draft.Title == nil || *out.Draft.Title != title {
		t.Errorf("expected the extracted fields to survive into the draft, got %+v", out.Draft)
	}
	if out.Draft.URL != "https://djinni.co/jobs/1-a/" {
		t.Errorf("draft url = %q", out.Draft.URL)
	}
}

func TestManualAdd_MissingURLIsRejectedWithoutCallingTheService(t *testing.T) {
	for _, body := range []map[string]any{{}, {"url": ""}, {"url": "   "}} {
		svc := &fakeAddService{}
		w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual", body, nil)
		if w.Code != 400 {
			t.Fatalf("status = %d for body %v, want 400", w.Code, body)
		}
		if svc.seen != "" {
			t.Errorf("expected the service not to be called for body %v", body)
		}
	}
}

func TestManualAdd_ServiceErrorIs500(t *testing.T) {
	svc := &fakeAddService{err: errors.New("database is on fire")}

	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual",
		map[string]any{"url": "https://djinni.co/jobs/1-a/"}, nil)

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestManualFillIn_CreatedIs201(t *testing.T) {
	svc := &fakeAddService{result: domain.Result{
		Outcome: domain.OutcomeCreated,
		Job:     &dto.JobDto{ID: "job-filled"},
	}}

	posted := "2026-07-28T00:00:00Z"
	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual/fill-in", map[string]any{
		"url":         "https://careers.example.com/jobs/42",
		"title":       "Senior Go Engineer",
		"company":     "NovaTech",
		"description": "Own the ledger service.",
		"remote":      true,
		"postedAt":    posted,
	}, nil)

	if w.Code != 201 {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if !svc.fillInOk {
		t.Fatal("expected the service to be called")
	}
	if svc.fillIn.PostedAt == nil || *svc.fillIn.PostedAt != posted {
		t.Errorf("expected postedAt to pass through untouched, got %v", svc.fillIn.PostedAt)
	}
	if svc.fillIn.SourceKey != nil {
		t.Errorf("expected an omitted sourceKey to stay nil so the service defaults it, got %v", *svc.fillIn.SourceKey)
	}
}

func TestManualFillIn_MissingFieldsAreNamed(t *testing.T) {
	svc := &fakeAddService{err: &application.MissingFieldsError{Fields: []string{"title", "description"}}}

	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual/fill-in", map[string]any{
		"url":     "https://careers.example.com/jobs/42",
		"company": "NovaTech",
	}, nil)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var out map[string]string
	testutil.ParseJSON(w, &out)
	if !strings.Contains(out["message"], "title") || !strings.Contains(out["message"], "description") {
		t.Errorf("expected every missing field to be named, got %q", out["message"])
	}
}

func TestManualFillIn_DuplicateIs200(t *testing.T) {
	svc := &fakeAddService{result: domain.Result{
		Outcome: domain.OutcomeDuplicate,
		Job:     &dto.JobDto{ID: "job-existing"},
	}}

	w := testutil.DoRequestJSON(router(svc), "POST", "/api/jobs/manual/fill-in", map[string]any{
		"url": "https://careers.example.com/jobs/42", "title": "t", "company": "c", "description": "d",
	}, nil)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
