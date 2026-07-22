package httpapi_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakeContactsProvider struct {
	list    []dto.JobContactDto
	listErr error

	refresh    []dto.JobContactDto
	refreshErr error
}

func (f *fakeContactsProvider) ListContacts(ctx context.Context, jobID string) ([]dto.JobContactDto, error) {
	return f.list, f.listErr
}

func (f *fakeContactsProvider) Resolve(ctx context.Context, jobID string) ([]dto.JobContactDto, error) {
	return f.refresh, f.refreshErr
}

func TestContactsList_Empty(t *testing.T) {
	h := &httpapi.ContactsHandler{Recruiter: &fakeContactsProvider{list: []dto.JobContactDto{}}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/contacts", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.JobContactDto
	testutil.ParseJSON(w, &out)
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d", len(out))
	}
}

func TestContactsList_Populated(t *testing.T) {
	title := "Recruiter"
	h := &httpapi.ContactsHandler{Recruiter: &fakeContactsProvider{
		list: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe", Title: &title, Source: "posting", Confidence: 0.9}},
	}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/contacts", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.JobContactDto
	testutil.ParseJSON(w, &out)
	if len(out) != 1 || out[0].Name != "Jane Doe" {
		t.Errorf("expected 1 contact named Jane Doe, got %+v", out)
	}
}

func TestContactsList_Error(t *testing.T) {
	h := &httpapi.ContactsHandler{Recruiter: &fakeContactsProvider{listErr: errFake}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job-1/contacts", nil, nil)
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestContactsRefresh_Success(t *testing.T) {
	h := &httpapi.ContactsHandler{Recruiter: &fakeContactsProvider{
		refresh: []dto.JobContactDto{{ID: "c1", Name: "Jane Doe", Source: "posting", Confidence: 0.9}},
	}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/jobs/job-1/contacts/refresh", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []dto.JobContactDto
	testutil.ParseJSON(w, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(out))
	}
}

func TestContactsRefresh_Error(t *testing.T) {
	h := &httpapi.ContactsHandler{Recruiter: &fakeContactsProvider{refreshErr: errFake}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/jobs/job-1/contacts/refresh", nil, nil)
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
