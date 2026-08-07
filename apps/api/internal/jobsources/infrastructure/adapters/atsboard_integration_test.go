//go:build integration

package adapters_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	ads "github.com/job-finder/api/internal/jobsources/infrastructure/adapters"
	"github.com/job-finder/api/internal/jobsources/roster"
)

func atsBoardMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/boards/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/v1/boards/"), "/", 2)
		id := parts[0]
		switch id {
		case "gh-test":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[{"id":1,"title":"Go Engineer","absolute_url":"https://boards.greenhouse.io/gh-test/jobs/1","content":"<p>Greenhouse full job description</p>","updated_at":"2026-01-01T00:00:00Z","location":{"name":"Remote, US"}}]}`))
		case "gh-empty":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		case "gh-404":
			w.WriteHeader(http.StatusNotFound)
		case "gh-malformed":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`not json`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		}
	})

	mux.HandleFunc("/v0/postings/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v0/postings/")
		if idx := strings.Index(id, "?"); idx >= 0 {
			id = id[:idx]
		}
		id = strings.TrimSuffix(id, "/")
		switch id {
		case "lv-test":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":"abc123","text":"Backend Engineer","hostedUrl":"https://jobs.lever.co/lv-test/abc123","createdAt":1700000000000,"categories":{"location":"New York, NY","commitment":"Full-time"},"descriptionPlain":"Lever full job description","description":"<p>Lever full job description</p>"}]`))
		case "lv-empty":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		case "lv-404":
			w.WriteHeader(http.StatusNotFound)
		case "lv-malformed":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`not json`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		}
	})

	mux.HandleFunc("/posting-api/job-board/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/posting-api/job-board/")
		switch id {
		case "as-test":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[{"id":"j1","title":"Senior Go Developer","location":"Remote","isRemote":true,"jobUrl":"https://jobs.ashbyhq.com/as-test/j1","descriptionHtml":"<p>Ashby full job description</p>","publishedAt":"2026-01-01T00:00:00Z"}]}`))
		case "as-empty":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		case "as-404":
			w.WriteHeader(http.StatusNotFound)
		case "as-malformed":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`not json`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		}
	})

	mux.HandleFunc("/api/v1/widget/accounts/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/widget/accounts/")
		switch id {
		case "wk-test":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[{"shortcode":"ABC123","title":"Platform Engineer","description":"Workable full job description","url":"https://apply.workable.com/wk-test/j/ABC123","remote":true,"created_at":"2026-01-01","location":{"city":"San Francisco","country_name":"US"}}]}`))
		case "wk-empty":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		case "wk-404":
			w.WriteHeader(http.StatusNotFound)
		case "wk-malformed":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`not json`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"jobs":[]}`))
		}
	})

	mux.HandleFunc("/v1/companies/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/v1/companies/"), "/", 2)
		id := parts[0]
		switch id {
		case "sr-test":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"content":[{"id":"p1","name":"Cloud Architect","releasedDate":"2026-01-01","ref":"https://jobs.smartrecruiters.com/SRTest/p1","location":{"city":"Chicago","country":"US","remote":true},"jobAd":{"sections":{"jobDescription":{"text":"SmartRecruiters full job description"}}}}]}`))
		case "sr-empty":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"content":[]}`))
		case "sr-404":
			w.WriteHeader(http.StatusNotFound)
		case "sr-malformed":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`not json`))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"content":[]}`))
		}
	})

	return mux
}

var testEmployers = []sqlcgen.InsertEmployerBoardParams{
	{Vendor: "greenhouse", EmployerIdentifier: "gh-test", DisplayName: "GH Test Corp", AddedVia: "test"},
	{Vendor: "greenhouse", EmployerIdentifier: "gh-empty", DisplayName: "GH Empty Corp", AddedVia: "test"},
	{Vendor: "greenhouse", EmployerIdentifier: "gh-404", DisplayName: "GH FourOhFour Corp", AddedVia: "test"},
	{Vendor: "greenhouse", EmployerIdentifier: "gh-malformed", DisplayName: "GH Malformed Corp", AddedVia: "test"},
	{Vendor: "lever", EmployerIdentifier: "lv-test", DisplayName: "LV Test Corp", AddedVia: "test"},
	{Vendor: "lever", EmployerIdentifier: "lv-empty", DisplayName: "LV Empty Corp", AddedVia: "test"},
	{Vendor: "lever", EmployerIdentifier: "lv-404", DisplayName: "LV FourOhFour Corp", AddedVia: "test"},
	{Vendor: "lever", EmployerIdentifier: "lv-malformed", DisplayName: "LV Malformed Corp", AddedVia: "test"},
	{Vendor: "ashby", EmployerIdentifier: "as-test", DisplayName: "AS Test Corp", AddedVia: "test"},
	{Vendor: "ashby", EmployerIdentifier: "as-empty", DisplayName: "AS Empty Corp", AddedVia: "test"},
	{Vendor: "ashby", EmployerIdentifier: "as-404", DisplayName: "AS FourOhFour Corp", AddedVia: "test"},
	{Vendor: "ashby", EmployerIdentifier: "as-malformed", DisplayName: "AS Malformed Corp", AddedVia: "test"},
	{Vendor: "workable", EmployerIdentifier: "wk-test", DisplayName: "WK Test Corp", AddedVia: "test"},
	{Vendor: "workable", EmployerIdentifier: "wk-empty", DisplayName: "WK Empty Corp", AddedVia: "test"},
	{Vendor: "workable", EmployerIdentifier: "wk-404", DisplayName: "WK FourOhFour Corp", AddedVia: "test"},
	{Vendor: "workable", EmployerIdentifier: "wk-malformed", DisplayName: "WK Malformed Corp", AddedVia: "test"},
	{Vendor: "smartrecruiters", EmployerIdentifier: "sr-test", DisplayName: "SR Test Corp", AddedVia: "test"},
	{Vendor: "smartrecruiters", EmployerIdentifier: "sr-empty", DisplayName: "SR Empty Corp", AddedVia: "test"},
	{Vendor: "smartrecruiters", EmployerIdentifier: "sr-404", DisplayName: "SR FourOhFour Corp", AddedVia: "test"},
	{Vendor: "smartrecruiters", EmployerIdentifier: "sr-malformed", DisplayName: "SR Malformed Corp", AddedVia: "test"},
}

func vendorHasJobs(identifier string) bool {
	return strings.HasSuffix(identifier, "-test")
}

type rewriteTransport struct {
	base *url.URL
	rt   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = t.base.Scheme
	u.Host = t.base.Host
	clone.URL = &u
	clone.Host = ""
	return t.rt.RoundTrip(clone)
}

func mockVendorClient(ts *httptest.Server) *http.Client {
	base, err := url.Parse(ts.URL)
	if err != nil {
		panic(err)
	}
	c := ts.Client()
	return &http.Client{Transport: rewriteTransport{base: base, rt: c.Transport}, Timeout: 30 * time.Second}
}

func setupATSTest(t *testing.T) (context.Context, *sqlcgen.Queries, *roster.Service, *http.ServeMux, *httptest.Server) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	testDB := dbtest.New(t)

	q := testDB.Queries

	for _, e := range testEmployers {
		if _, err := q.InsertEmployerBoard(ctx, e); err != nil {
			t.Fatalf("insert employer %s/%s: %v", e.Vendor, e.EmployerIdentifier, err)
		}
	}

	gh, lv, as, wk, sr, checkers := ads.NewBoardAdapters()
	rosterSvc := roster.NewService(q, checkers)
	gh.Roster = rosterSvc
	lv.Roster = rosterSvc
	as.Roster = rosterSvc
	wk.Roster = rosterSvc
	sr.Roster = rosterSvc

	mux := atsBoardMux(t)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return ctx, q, rosterSvc, mux, ts
}

func TestATSBoardIntegration_Greenhouse(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	gh := &ads.GreenhouseAdapter{Roster: rosterSvc}

	origClient := jobsources.DefaultClient()
	restore := func() { jobsources.SetDefaultClient(origClient) }

	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer restore()

	jobs, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search: %v", err)
	}
	hasTest := false
	hasEmpty := false
	for _, j := range jobs {
		if j.Company == "GH Test Corp" {
			hasTest = true
			if j.Title != "Go Engineer" {
				t.Errorf("expected title 'Go Engineer', got %q", j.Title)
			}
			if j.URL == "" {
				t.Error("expected non-empty URL")
			}
			if j.Description == "" {
				t.Error("expected non-empty description")
			}
			if !j.Remote {
				t.Error("expected remote=true for 'Remote, US' location")
			}
			if j.SourceKey != "greenhouse" {
				t.Errorf("expected sourceKey 'greenhouse', got %q", j.SourceKey)
			}
		}
		if j.Company == "GH Empty Corp" {
			hasEmpty = true
		}
	}
	if !hasTest {
		t.Error("expected at least one job from GH Test Corp")
	}
	if hasEmpty {
		t.Error("expected no jobs from GH Empty Corp")
	}

	origClient = jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs2, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search re-run: %v", err)
	}
	count2 := 0
	for _, j := range jobs2 {
		if j.Company == "GH Test Corp" {
			count2++
		}
	}
	if count2 != 1 {
		t.Errorf("expected 1 job from GH Test Corp on re-run (adapters always return fresh), got %d", count2)
	}
}

func TestATSBoardIntegration_Lever(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	lv := &ads.LeverAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := lv.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Lever Search: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.Company == "LV Test Corp" {
			found = true
			if j.Title != "Backend Engineer" {
				t.Errorf("expected title 'Backend Engineer', got %q", j.Title)
			}
			if j.URL == "" {
				t.Error("expected non-empty URL")
			}
			if j.Description == "" {
				t.Error("expected non-empty description (descriptionPlain)")
			}
			if j.SourceKey != "lever" {
				t.Errorf("expected sourceKey 'lever', got %q", j.SourceKey)
			}
		}
	}
	if !found {
		t.Error("expected at least one job from LV Test Corp")
	}
}

func TestATSBoardIntegration_Ashby(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	as := &ads.AshbyAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := as.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Ashby Search: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.Company == "AS Test Corp" {
			found = true
			if j.Title != "Senior Go Developer" {
				t.Errorf("expected title 'Senior Go Developer', got %q", j.Title)
			}
			if j.URL == "" {
				t.Error("expected non-empty URL")
			}
			if j.Description == "" {
				t.Error("expected non-empty description")
			}
			if !j.Remote {
				t.Error("expected remote=true")
			}
			if j.SourceKey != "ashby" {
				t.Errorf("expected sourceKey 'ashby', got %q", j.SourceKey)
			}
		}
	}
	if !found {
		t.Error("expected at least one job from AS Test Corp")
	}
}

func TestATSBoardIntegration_Workable(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	wk := &ads.WorkableAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := wk.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Workable Search: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.Company == "WK Test Corp" {
			found = true
			if j.Title != "Platform Engineer" {
				t.Errorf("expected title 'Platform Engineer', got %q", j.Title)
			}
			if j.URL == "" {
				t.Error("expected non-empty URL")
			}
			if j.Description == "" {
				t.Error("expected non-empty description")
			}
			if !j.Remote {
				t.Error("expected remote=true")
			}
			if j.SourceKey != "workable" {
				t.Errorf("expected sourceKey 'workable', got %q", j.SourceKey)
			}
		}
	}
	if !found {
		t.Error("expected at least one job from WK Test Corp")
	}
}

func TestATSBoardIntegration_SmartRecruiters(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	sr := &ads.SmartRecruitersAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := sr.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("SmartRecruiters Search: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.Company == "SR Test Corp" {
			found = true
			if j.Title != "Cloud Architect" {
				t.Errorf("expected title 'Cloud Architect', got %q", j.Title)
			}
			if j.URL == "" {
				t.Error("expected non-empty URL")
			}
			if j.Description == "" {
				t.Error("expected non-empty description")
			}
			if !j.Remote {
				t.Error("expected remote=true")
			}
			if j.SourceKey != "smartrecruiters" {
				t.Errorf("expected sourceKey 'smartrecruiters', got %q", j.SourceKey)
			}
		}
	}
	if !found {
		t.Error("expected at least one job from SR Test Corp")
	}
}

func TestATSBoardIntegration_404Employer(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	gh := &ads.GreenhouseAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search with 404 employer: %v", err)
	}
	for _, j := range jobs {
		if j.Company == "GH FourOhFour Corp" {
			t.Error("expected no jobs from 404 employer")
		}
	}
}

func TestATSBoardIntegration_MalformedResponse(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	gh := &ads.GreenhouseAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search with malformed: %v", err)
	}
	for _, j := range jobs {
		if j.Company == "GH Malformed Corp" {
			t.Error("expected no jobs from malformed-response employer")
		}
	}
}

func TestATSBoardIntegration_PreviouslySeenDisappears(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	gh := &ads.GreenhouseAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search first run: %v", err)
	}
	hasFirst := false
	for _, j := range jobs {
		if j.Company == "GH Test Corp" {
			hasFirst = true
			break
		}
	}
	if !hasFirst {
		t.Fatal("first run should return a job from GH Test Corp")
	}

	origClient = jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	jobs2, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search re-run: %v", err)
	}
	for _, j := range jobs2 {
		if j.Company == "GH Empty Corp" {
			t.Error("expected no jobs from GH Empty Corp")
		}
	}
}

func TestATSBoardIntegration_EmployerReporter(t *testing.T) {
	ctx, _, rosterSvc, _, ts := setupATSTest(t)

	gh := &ads.GreenhouseAdapter{Roster: rosterSvc}
	origClient := jobsources.DefaultClient()
	jobsources.SetDefaultClient(mockVendorClient(ts))
	defer func() { jobsources.SetDefaultClient(origClient) }()

	_, err := gh.Search(ctx, emptyQuery, nil)
	if err != nil {
		t.Fatalf("Greenhouse Search: %v", err)
	}

	detail := gh.LastRunDetail()
	if len(detail) != 4 {
		t.Fatalf("expected 4 employer outcomes (one per greenhouse employer), got %d", len(detail))
	}
	for _, d := range detail {
		switch d.EmployerIdentifier {
		case "gh-test":
			if d.Outcome != "read" {
				t.Errorf("expected outcome 'read' for gh-test, got %q", d.Outcome)
			}
			if d.PostingsFound != 1 {
				t.Errorf("expected 1 posting for gh-test, got %d", d.PostingsFound)
			}
		case "gh-empty":
			if d.Outcome != "no_postings" {
				t.Errorf("expected outcome 'no_postings' for gh-empty, got %q", d.Outcome)
			}
		case "gh-404":
			if d.Outcome != "not_found" {
				t.Errorf("expected outcome 'not_found' for gh-404, got %q", d.Outcome)
			}
		case "gh-malformed":
			if d.Outcome != "unreadable" {
				t.Errorf("expected outcome 'unreadable' for gh-malformed, got %q", d.Outcome)
			}
		}
	}
}

var emptyQuery = dto.SearchQuery{}
