package recruiter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/llm"
)

// multiSourceFakeLLM lets a test drive posting vs. company-page/LinkedIn
// extraction independently, since both go through the same llm.Provider —
// it distinguishes them by the prompt marker each source uses.
type multiSourceFakeLLM struct {
	postingJSON string
	pageJSON    string
	pageErr     error
}

func (m *multiSourceFakeLLM) ModelName() string { return "test-model" }
func (m *multiSourceFakeLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (m *multiSourceFakeLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	if strings.Contains(prompt, "PAGE TEXT") {
		if m.pageErr != nil {
			return "", m.pageErr
		}
		return m.pageJSON, nil
	}
	return m.postingJSON, nil
}
func (m *multiSourceFakeLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

// fakeRepository is an in-memory Repository. UpsertJobContact mirrors the
// real (jobId, source, name) upsert-in-place semantics (FR-013) so
// orchestration-level idempotency can be asserted without a live DB — the
// SQL-layer constraint itself is covered by
// apps/api/internal/db/integration_test.go's TestJobContactUpsertIdempotent.
type fakeRepository struct {
	job      sqlcgen.Job
	jobErr   error
	company  sqlcgen.Company
	compErr  error
	contacts map[string]sqlcgen.JobContact // key: source|name
	nextID   int
}

func newFakeRepository(job sqlcgen.Job) *fakeRepository {
	return &fakeRepository{job: job, contacts: map[string]sqlcgen.JobContact{}, compErr: pgx.ErrNoRows}
}

func (f *fakeRepository) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return f.job, f.jobErr
}

func (f *fakeRepository) GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error) {
	return f.company, f.compErr
}

func (f *fakeRepository) UpsertJobContact(ctx context.Context, arg sqlcgen.UpsertJobContactParams) (sqlcgen.JobContact, error) {
	key := arg.Source + "|" + arg.Name
	if existing, ok := f.contacts[key]; ok {
		existing.Title = arg.Title
		existing.LinkedInUrl = arg.LinkedInUrl
		existing.Email = arg.Email
		existing.Phone = arg.Phone
		existing.Confidence = arg.Confidence
		f.contacts[key] = existing
		return existing, nil
	}
	f.nextID++
	row := sqlcgen.JobContact{
		ID:          pgtype.UUID{Bytes: [16]byte{byte(f.nextID)}, Valid: true},
		JobId:       arg.JobId,
		Name:        arg.Name,
		Title:       arg.Title,
		LinkedInUrl: arg.LinkedInUrl,
		Email:       arg.Email,
		Phone:       arg.Phone,
		Source:      arg.Source,
		Confidence:  arg.Confidence,
	}
	f.contacts[key] = row
	return row, nil
}

func (f *fakeRepository) ListJobContactsByJob(ctx context.Context, jobId pgtype.UUID) ([]sqlcgen.JobContact, error) {
	out := make([]sqlcgen.JobContact, 0, len(f.contacts))
	for _, c := range f.contacts {
		out = append(out, c)
	}
	// Mirror the real ListJobContactsByJob SQL ORDER BY (confidence desc,
	// source priority, name asc) so orchestration-level tests can assert
	// deterministic ordering without a live DB — the SQL itself is covered
	// by apps/api/internal/db/integration_test.go's TestJobContactOrdering.
	sourcePriority := map[string]int{SourcePosting: 0, SourceCompanyPage: 1, SourceLinkedIn: 2}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		pi, pj := sourcePriority[out[i].Source], sourcePriority[out[j].Source]
		if pi != pj {
			return pi < pj
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// fakeScraping is a ScrapingService that records every URL it was asked to
// fetch and returns a canned/erroring response per URL substring.
type fakeScraping struct {
	responses map[string]string
	errs      map[string]error
	requested []string
}

func (f *fakeScraping) FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error) {
	f.requested = append(f.requested, url)
	for substr, err := range f.errs {
		if strings.Contains(url, substr) {
			return "", err
		}
	}
	for substr, html := range f.responses {
		if strings.Contains(url, substr) {
			return html, nil
		}
	}
	return "", fmt.Errorf("fakeScraping: no fixture for %s", url)
}

func testJob(description string) sqlcgen.Job {
	return sqlcgen.Job{
		ID:          pgtype.UUID{Bytes: [16]byte{9}, Valid: true},
		Company:     "Acme Corp",
		Description: description,
	}
}

// TestLinkedInSkippedWhenDisabled covers T017/FR-004/SC-004: with
// linkedInEnabled=false, the LinkedIn source is never invoked — zero
// requests to any linkedin.com URL are made — and the run still completes.
func TestLinkedInSkippedWhenDisabled(t *testing.T) {
	job := testJob("We are hiring. Contact: Jane Doe, Recruiter <jane@acme.com>")
	repo := newFakeRepository(job)
	website := "https://acme.example.com"
	repo.company = sqlcgen.Company{Website: &website}
	repo.compErr = nil

	scraping := &fakeScraping{
		responses: map[string]string{"acme.example.com": `<html><body>About Acme</body></html>`},
	}
	llmc := &fakeLLM{json: `{"name":"Jane Doe","title":"Recruiter","email":"jane@acme.com","phone":"","linkedInUrl":""}`}

	svc := NewService(repo, llmc, "", scraping, false)
	contacts, err := svc.Resolve(context.Background(), dbutil.UUIDString(job.ID))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(contacts) == 0 {
		t.Fatal("expected the posting contact to still be resolved")
	}
	for _, url := range scraping.requested {
		if strings.Contains(url, "linkedin.com") {
			t.Fatalf("expected zero LinkedIn requests when disabled, got a request to %s", url)
		}
	}
}

// TestLinkedInRunsWhenEnabled is the mirror check: with the flag on, a
// LinkedIn request IS made.
func TestLinkedInRunsWhenEnabled(t *testing.T) {
	job := testJob("We are hiring a backend engineer.")
	repo := newFakeRepository(job)

	scraping := &fakeScraping{
		responses: map[string]string{"linkedin.com": `<html><body>People at Acme Corp</body></html>`},
	}
	llmc := &fakeLLM{json: `{"contacts":[]}`}

	svc := NewService(repo, llmc, "", scraping, true)
	if _, err := svc.Resolve(context.Background(), dbutil.UUIDString(job.ID)); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	found := false
	for _, url := range scraping.requested {
		if strings.Contains(url, "linkedin.com") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a LinkedIn request when LinkedInScrapeEnabled=true")
	}
}

// TestResolveOneSourceFails covers T018/FR-015/SC-007: a failing source
// (the company-page extraction call itself errors, after a successful
// fetch) does not stop the other sources' contacts from being persisted,
// and Resolve itself returns no error — the failure is logged and
// isolated to that one source.
func TestResolveOneSourceFails(t *testing.T) {
	job := testJob("Contact: Jane Doe, Recruiter <jane@acme.com>")
	repo := newFakeRepository(job)
	website := "https://acme.example.com"
	repo.company = sqlcgen.Company{Website: &website}
	repo.compErr = nil

	scraping := &fakeScraping{
		responses: map[string]string{"acme.example.com": `<html><body>Team page</body></html>`},
	}
	llmc := &multiSourceFakeLLM{
		postingJSON: `{"name":"Jane Doe","title":"Recruiter","email":"jane@acme.com","phone":"","linkedInUrl":""}`,
		pageErr:     fmt.Errorf("model unavailable"),
	}

	svc := NewService(repo, llmc, "", scraping, false)
	contacts, err := svc.Resolve(context.Background(), dbutil.UUIDString(job.ID))
	if err != nil {
		t.Fatalf("Resolve should not fail when only one source errors: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected the posting contact to survive a company-page failure, got %d contacts", len(contacts))
	}
	if contacts[0].Name != "Jane Doe" {
		t.Errorf("Name = %q, want Jane Doe", contacts[0].Name)
	}
	if contacts[0].Source != SourcePosting {
		t.Errorf("Source = %q, want %q", contacts[0].Source, SourcePosting)
	}
}

// TestResolveIdempotent covers T019/FR-013/SC-006 at the orchestration
// level: two Resolve runs against unchanged source data leave the contact
// count unchanged (the fake repository mirrors the DB's upsert-in-place
// semantics on (jobId, source, name)).
func TestResolveIdempotent(t *testing.T) {
	job := testJob("Contact: Jane Doe, Recruiter <jane@acme.com>")
	repo := newFakeRepository(job)
	scraping := &fakeScraping{}
	llmc := &fakeLLM{json: `{"name":"Jane Doe","title":"Recruiter","email":"jane@acme.com","phone":"","linkedInUrl":""}`}

	svc := NewService(repo, llmc, "", scraping, false)

	first, err := svc.Resolve(context.Background(), dbutil.UUIDString(job.ID))
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	second, err := svc.Resolve(context.Background(), dbutil.UUIDString(job.ID))
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("expected stable row count across re-runs, got %d then %d", len(first), len(second))
	}
	if len(repo.contacts) != len(first) {
		t.Fatalf("expected no duplicate rows in the repository, got %d stored vs %d listed", len(repo.contacts), len(first))
	}
}

// TestListOrderingDeterministic covers T025/FR-010/SC-010: contacts from
// multiple sources sort by confidence desc with the stable
// source-priority/name tie-break, and repeated reads of unchanged data
// return the identical order.
func TestListOrderingDeterministic(t *testing.T) {
	job := testJob("n/a")
	repo := newFakeRepository(job)

	seed := []struct {
		name, source string
		confidence   float64
	}{
		{"B Person", SourceLinkedIn, 0.5},
		{"A Person", SourcePosting, 0.5},
		{"C Person", SourcePosting, 0.9},
		{"D Person", SourceCompanyPage, 0.5},
	}
	for _, s := range seed {
		if _, err := repo.UpsertJobContact(context.Background(), sqlcgen.UpsertJobContactParams{
			JobId: job.ID, Name: s.name, Source: s.source, Confidence: s.confidence,
		}); err != nil {
			t.Fatalf("seed upsert: %v", err)
		}
	}

	svc := NewService(repo, &fakeLLM{json: `{}`}, "", &fakeScraping{}, false)

	want := []string{"C Person", "A Person", "D Person", "B Person"}
	for read := 0; read < 3; read++ {
		got, err := svc.ListContacts(context.Background(), dbutil.UUIDString(job.ID))
		if err != nil {
			t.Fatalf("ListContacts (read %d): %v", read, err)
		}
		if len(got) != len(want) {
			t.Fatalf("read %d: expected %d contacts, got %d", read, len(want), len(got))
		}
		for i, name := range want {
			if got[i].Name != name {
				t.Fatalf("read %d, position %d: expected %s, got %s", read, i, name, got[i].Name)
			}
		}
	}
}
