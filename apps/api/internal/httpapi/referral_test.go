package httpapi_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/referral"
	"github.com/job-finder/api/internal/testutil"
)

type fakeReferralProvider struct {
	importSummary referral.ImportSummary
	importErr     error
	syncResult    *referral.GithubSyncResult
	syncErr       error
	paths         []referral.ReferralPath
	pathsErr      error
	contacts      []referral.Contact
	contactsErr   error
}

func (f *fakeReferralProvider) ListContacts(ctx context.Context) ([]referral.Contact, error) {
	return f.contacts, f.contactsErr
}

func (f *fakeReferralProvider) ImportCSV(ctx context.Context, r io.Reader) (referral.ImportSummary, error) {
	return f.importSummary, f.importErr
}

func (f *fakeReferralProvider) SyncGithub(ctx context.Context, contactID string) (*referral.GithubSyncResult, error) {
	return f.syncResult, f.syncErr
}

func (f *fakeReferralProvider) FindReferralPaths(ctx context.Context, jobID string, maxDepth, topN int) ([]referral.ReferralPath, error) {
	return f.paths, f.pathsErr
}

func TestReferralListContacts(t *testing.T) {
	f := &fakeReferralProvider{contacts: []referral.Contact{{ID: "1", Name: "Jane"}}}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/contacts", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []map[string]any
	testutil.ParseJSON(w, &out)
	if len(out) != 1 || out[0]["name"] != "Jane" {
		t.Errorf("got %+v, want one contact named Jane", out)
	}
}

func TestReferralImportCSV(t *testing.T) {
	f := &fakeReferralProvider{importSummary: referral.ImportSummary{Imported: 2, Total: 2}}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/contacts/import", strings.NewReader("Name\nJane\n"), nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]int
	testutil.ParseJSON(w, &out)
	if out["imported"] != 2 {
		t.Errorf("imported = %d, want 2", out["imported"])
	}
}

func TestReferralImportCSV_Error(t *testing.T) {
	f := &fakeReferralProvider{importErr: errors.New("bad csv")}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/contacts/import", strings.NewReader(""), nil)
	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestReferralGithubSync(t *testing.T) {
	f := &fakeReferralProvider{syncResult: &referral.GithubSyncResult{ConnectionsMade: 3}}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/contacts/c1/github-sync", nil, map[string]string{"id": "c1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	testutil.ParseJSON(w, &out)
	if out["connectionsMade"].(float64) != 3 {
		t.Errorf("connectionsMade = %v, want 3", out["connectionsMade"])
	}
}

func TestReferralGithubSync_NotFound(t *testing.T) {
	f := &fakeReferralProvider{syncErr: referral.ErrContactNotFound}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/contacts/missing/github-sync", nil, map[string]string{"id": "missing"})
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestReferralGithubSync_NoUsername(t *testing.T) {
	f := &fakeReferralProvider{syncErr: referral.ErrNoGithubUsername}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/contacts/c1/github-sync", nil, map[string]string{"id": "c1"})
	if w.Code != 422 {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

func TestReferralPaths(t *testing.T) {
	f := &fakeReferralProvider{paths: []referral.ReferralPath{
		{Path: []referral.Contact{{ID: "1", Name: "Me"}, {ID: "2", Name: "Target"}}, Score: 0.7, Length: 2},
	}}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job1/referral-paths", nil, map[string]string{"id": "job1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out []map[string]any
	testutil.ParseJSON(w, &out)
	if len(out) != 1 {
		t.Fatalf("expected 1 path, got %d", len(out))
	}
	if out[0]["score"].(float64) != 0.7 {
		t.Errorf("score = %v, want 0.7", out[0]["score"])
	}
}

func TestReferralPaths_Error(t *testing.T) {
	f := &fakeReferralProvider{pathsErr: errors.New("boom")}
	h := &httpapi.ReferralHandler{Referral: f}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/jobs/job1/referral-paths", nil, map[string]string{"id": "job1"})
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
