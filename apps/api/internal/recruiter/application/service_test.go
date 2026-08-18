package application

import (
	"context"
	"fmt"
	"github.com/job-finder/api/internal/recruiter/domain"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

// multiSourceFakeExtractor stands in for a ContactExtractor whose result
// depends on which of the three sources (posting/company_page/linkedin) is
// asking — recruiter's Go LLM path is deleted (T113), so this only needs to
// mimic AIContactExtractor's per-source shape, not any prompt text.
type multiSourceFakeExtractor struct {
	posting []ExtractedContact
	page    []ExtractedContact
	pageErr error
}

func (m *multiSourceFakeExtractor) Extract(ctx context.Context, source, text string) ([]ExtractedContact, error) {
	if source == SourcePosting {
		return m.posting, nil
	}
	if m.pageErr != nil {
		return nil, m.pageErr
	}
	return m.page, nil
}

type fakeRepository struct {
	job      sqlcgen.Job
	jobErr   error
	company  sqlcgen.Company
	compErr  error
	contacts map[string]sqlcgen.JobContact
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
	sourcePriority := map[string]int{domain.SourcePosting: 0, domain.SourceCompanyPage: 1, domain.SourceLinkedIn: 2}
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

func TestLinkedInSkippedWhenDisabled(t *testing.T) {
	job := testJob("We are hiring. Contact: Jane Doe, Recruiter <jane@acme.com>")
	repo := newFakeRepository(job)
	website := "https://acme.example.com"
	repo.company = sqlcgen.Company{Website: &website}
	repo.compErr = nil

	scraping := &fakeScraping{
		responses: map[string]string{"acme.example.com": `<html><body>About Acme</body></html>`},
	}
	extractor := &fakeExtractor{contacts: []ExtractedContact{{Name: "Jane Doe", Title: "Recruiter", Email: "jane@acme.com"}}}

	svc := NewService(repo, extractor, scraping, false)
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

func TestLinkedInRunsWhenEnabled(t *testing.T) {
	job := testJob("We are hiring a backend engineer.")
	repo := newFakeRepository(job)

	scraping := &fakeScraping{
		responses: map[string]string{"linkedin.com": `<html><body>People at Acme Corp</body></html>`},
	}
	extractor := &fakeExtractor{}

	svc := NewService(repo, extractor, scraping, true)
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

func TestResolveOneSourceFails(t *testing.T) {
	job := testJob("Contact: Jane Doe, Recruiter <jane@acme.com>")
	repo := newFakeRepository(job)
	website := "https://acme.example.com"
	repo.company = sqlcgen.Company{Website: &website}
	repo.compErr = nil

	scraping := &fakeScraping{
		responses: map[string]string{"acme.example.com": `<html><body>Team page</body></html>`},
	}
	extractor := &multiSourceFakeExtractor{
		posting: []ExtractedContact{{Name: "Jane Doe", Title: "Recruiter", Email: "jane@acme.com"}},
		pageErr: fmt.Errorf("model unavailable"),
	}

	svc := NewService(repo, extractor, scraping, false)
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
	if contacts[0].Source != domain.SourcePosting {
		t.Errorf("Source = %q, want %q", contacts[0].Source, domain.SourcePosting)
	}
}

func TestResolveIdempotent(t *testing.T) {
	job := testJob("Contact: Jane Doe, Recruiter <jane@acme.com>")
	repo := newFakeRepository(job)
	scraping := &fakeScraping{}
	extractor := &fakeExtractor{contacts: []ExtractedContact{{Name: "Jane Doe", Title: "Recruiter", Email: "jane@acme.com"}}}

	svc := NewService(repo, extractor, scraping, false)

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

func TestListOrderingDeterministic(t *testing.T) {
	job := testJob("n/a")
	repo := newFakeRepository(job)

	seed := []struct {
		name, source string
		confidence   float64
	}{
		{"B Person", domain.SourceLinkedIn, 0.5},
		{"A Person", domain.SourcePosting, 0.5},
		{"C Person", domain.SourcePosting, 0.9},
		{"D Person", domain.SourceCompanyPage, 0.5},
	}
	for _, s := range seed {
		if _, err := repo.UpsertJobContact(context.Background(), sqlcgen.UpsertJobContactParams{
			JobId: job.ID, Name: s.name, Source: s.source, Confidence: s.confidence,
		}); err != nil {
			t.Fatalf("seed upsert: %v", err)
		}
	}

	svc := NewService(repo, &fakeExtractor{}, &fakeScraping{}, false)

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
