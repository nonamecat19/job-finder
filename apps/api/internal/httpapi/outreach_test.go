package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/outreach"
	"github.com/job-finder/api/internal/testutil"
)

type fakeOutreachProvider struct {
	draft dto.OutreachDraftDto
	err   error
	tones []dto.OutreachToneOptionDto
}

func (f *fakeOutreachProvider) GenerateDraft(ctx context.Context, jobID, contactID, tone string) (dto.OutreachDraftDto, error) {
	if f.err != nil {
		return dto.OutreachDraftDto{}, f.err
	}
	f.draft.JobID = jobID
	return f.draft, nil
}

func (f *fakeOutreachProvider) Tones() []dto.OutreachToneOptionDto {
	return f.tones
}

func sampleDraft() dto.OutreachDraftDto {
	name := "Jane Doe"
	id := "c1"
	return dto.OutreachDraftDto{
		ContactID:   &id,
		ContactName: &name,
		Tone:        "warm",
		Text:        "Hi Jane, ...",
		GroundingTraces: []dto.GroundingTraceDto{
			{Claim: "Go, React, Postgres", SignalKind: "tech_stack", SignalValue: "Go, React, Postgres"},
		},
		GeneratedAt: "2026-07-22T00:00:00Z",
	}
}

func TestOutreachGenerate(t *testing.T) {
	h := &httpapi.OutreachHandler{Outreach: &fakeOutreachProvider{draft: sampleDraft()}}
	r := testutil.SetupRouter(h.Mount)

	body := map[string]string{"contactId": "c1", "tone": "warm"}
	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/outreach/generate", body, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out dto.OutreachDraftDto
	testutil.ParseJSON(w, &out)
	if out.JobID != "job-1" {
		t.Errorf("expected jobId echoed, got %q", out.JobID)
	}
	if out.ContactName == nil || *out.ContactName != "Jane Doe" {
		t.Errorf("expected contact name, got %v", out.ContactName)
	}
	if len(out.GroundingTraces) != 1 {
		t.Errorf("expected 1 grounding trace, got %d", len(out.GroundingTraces))
	}
}

func TestOutreachGenerate_NoBody(t *testing.T) {
	h := &httpapi.OutreachHandler{Outreach: &fakeOutreachProvider{draft: sampleDraft()}}
	r := testutil.SetupRouter(h.Mount)

	// Contact/tone are optional (empty contactId picks the sole contact or
	// requires a choice; empty tone defaults) — a body-less POST must not
	// 400.
	w := testutil.DoRequest(r, "POST", "/api/jobs/job-1/outreach/generate", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOutreachGenerate_ContactNotFound(t *testing.T) {
	h := &httpapi.OutreachHandler{Outreach: &fakeOutreachProvider{err: outreach.ErrContactNotFound}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/outreach/generate", map[string]string{"contactId": "nope"}, map[string]string{"id": "job-1"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOutreachGenerate_ContactRequired(t *testing.T) {
	h := &httpapi.OutreachHandler{Outreach: &fakeOutreachProvider{err: outreach.ErrContactRequired}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/outreach/generate", map[string]string{}, map[string]string{"id": "job-1"})
	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOutreachTones(t *testing.T) {
	tones := []dto.OutreachToneOptionDto{
		{Value: "warm", Label: "Warm", Default: true},
		{Value: "direct", Label: "Direct"},
		{Value: "formal", Label: "Formal"},
	}
	h := &httpapi.OutreachHandler{Outreach: &fakeOutreachProvider{tones: tones}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/outreach/tones", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var out []dto.OutreachToneOptionDto
	testutil.ParseJSON(w, &out)
	if len(out) != 3 {
		t.Fatalf("expected 3 tones, got %d", len(out))
	}
}
