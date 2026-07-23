package httpapi_test

import (
	"context"

	"github.com/job-finder/api/internal/companyintel/application"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"

	"testing"
)

type fakeCompanyIntelProvider struct {
	intel      *dto.CompanyIntelDto
	intelErr   error
	refresh    *dto.CompanyIntelDto
	refreshErr error
}

func (f *fakeCompanyIntelProvider) GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error) {
	return f.intel, f.intelErr
}

func (f *fakeCompanyIntelProvider) Refresh(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error) {
	return f.refresh, f.refreshErr
}

func TestCompaniesIntel_NoData(t *testing.T) {
	h := &httpapi.CompaniesHandler{CompanyIntel: &fakeCompanyIntelProvider{}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/companies/job-1/intel", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "null\n" && body != "null" {
		t.Errorf("expected a JSON null body for no-data-yet, got %q", body)
	}
}

func TestCompaniesIntel_Populated(t *testing.T) {
	funding := "Series B"
	h := &httpapi.CompaniesHandler{CompanyIntel: &fakeCompanyIntelProvider{
		intel: &dto.CompanyIntelDto{CompanyName: "Acme Corp", Funding: &funding, FetchedAt: "2026-01-01T00:00:00Z"},
	}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/companies/job-1/intel", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out dto.CompanyIntelDto
	testutil.ParseJSON(w, &out)
	if out.CompanyName != "Acme Corp" {
		t.Errorf("CompanyName = %q, want %q", out.CompanyName, "Acme Corp")
	}
	if out.Funding == nil || *out.Funding != "Series B" {
		t.Errorf("Funding = %v, want %q", out.Funding, "Series B")
	}
}

func TestCompaniesIntel_Error(t *testing.T) {
	h := &httpapi.CompaniesHandler{CompanyIntel: &fakeCompanyIntelProvider{intelErr: errFake}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/companies/job-1/intel", nil, nil)
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCompaniesRefresh_Success(t *testing.T) {
	h := &httpapi.CompaniesHandler{CompanyIntel: &fakeCompanyIntelProvider{
		refresh: &dto.CompanyIntelDto{CompanyName: "Acme Corp", FetchedAt: "2026-01-01T00:00:00Z"},
	}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/companies/job-1/intel/refresh", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCompaniesRefresh_NoCompany(t *testing.T) {
	h := &httpapi.CompaniesHandler{CompanyIntel: &fakeCompanyIntelProvider{refreshErr: application.ErrNoCompany}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "POST", "/api/companies/job-1/intel/refresh", nil, nil)
	if w.Code != 422 {
		t.Fatalf("expected 422, got %d", w.Code)
	}
}

var errFake = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }
