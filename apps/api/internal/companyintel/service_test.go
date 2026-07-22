package companyintel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/companyintel"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

const testJobID = "11111111-1111-1111-1111-111111111111"
const testCompanyID = "22222222-2222-2222-2222-222222222222"

// fakeRepo embeds the Repository port; the test overrides only what it needs.
type fakeRepo struct {
	companyintel.Repository

	job    sqlcgen.Job
	jobErr error

	company    sqlcgen.Company
	companyErr error

	signals    []sqlcgen.CompanySignal
	signalsErr error

	signalByKind    sqlcgen.CompanySignal
	signalByKindErr error

	upserted []sqlcgen.UpsertCompanySignalParams
}

func (f *fakeRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return f.job, f.jobErr
}

func (f *fakeRepo) GetCompanyByNormalizedName(ctx context.Context, normalizedName string) (sqlcgen.Company, error) {
	return f.company, f.companyErr
}

func (f *fakeRepo) UpsertCompany(ctx context.Context, arg sqlcgen.UpsertCompanyParams) (sqlcgen.Company, error) {
	return f.company, nil
}

func (f *fakeRepo) UpdateCompanyLastRefreshed(ctx context.Context, id pgtype.UUID) error {
	return nil
}

func (f *fakeRepo) GetCompanySignals(ctx context.Context, companyId pgtype.UUID) ([]sqlcgen.CompanySignal, error) {
	return f.signals, f.signalsErr
}

func (f *fakeRepo) GetCompanySignalByKind(ctx context.Context, arg sqlcgen.GetCompanySignalByKindParams) (sqlcgen.CompanySignal, error) {
	return f.signalByKind, f.signalByKindErr
}

func (f *fakeRepo) UpsertCompanySignal(ctx context.Context, arg sqlcgen.UpsertCompanySignalParams) (sqlcgen.CompanySignal, error) {
	f.upserted = append(f.upserted, arg)
	return sqlcgen.CompanySignal{ID: arg.CompanyId, CompanyId: arg.CompanyId, Kind: arg.Kind, Value: arg.Value}, nil
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}
	return u
}

func jsonValue(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// fakeScraper is a deterministic test double for Scraper.
type fakeScraper struct {
	kind, domain string
	result       *companyintel.SignalResult
	err          error
}

func (f fakeScraper) Kind() string   { return f.kind }
func (f fakeScraper) Domain() string { return f.domain }
func (f fakeScraper) Scrape(ctx context.Context, in companyintel.Input) (*companyintel.SignalResult, error) {
	return f.result, f.err
}

func TestGetIntel_NoCompanyName(t *testing.T) {
	repo := &fakeRepo{job: sqlcgen.Job{Company: "   "}}
	svc := companyintel.NewService(repo, companyintel.NewRegistry(), time.Millisecond)

	out, err := svc.GetIntel(context.Background(), testJobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil dto for a job with no company name, got %+v", out)
	}
}

func TestGetIntel_CompanyNeverProbed(t *testing.T) {
	repo := &fakeRepo{
		job:        sqlcgen.Job{Company: "Acme Corp"},
		companyErr: pgx.ErrNoRows,
	}
	svc := companyintel.NewService(repo, companyintel.NewRegistry(), time.Millisecond)

	out, err := svc.GetIntel(context.Background(), testJobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil dto for a never-probed company, got %+v", out)
	}
}

func TestGetIntel_ReturnsFlattenedSignals(t *testing.T) {
	companyID := mustUUID(t, testCompanyID)
	repo := &fakeRepo{
		job:     sqlcgen.Job{Company: "Acme Corp"},
		company: sqlcgen.Company{ID: companyID, Name: "Acme Corp"},
		signals: []sqlcgen.CompanySignal{
			{CompanyId: companyID, Kind: companyintel.KindFunding, Value: jsonValue(t, "Series B — $50M"), FetchedAt: pgtype.Timestamp{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}},
			{CompanyId: companyID, Kind: companyintel.KindGlassdoorRating, Value: jsonValue(t, 3.8), FetchedAt: pgtype.Timestamp{Time: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Valid: true}},
		},
	}
	svc := companyintel.NewService(repo, companyintel.NewRegistry(), time.Millisecond)

	out, err := svc.GetIntel(context.Background(), testJobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected a populated dto")
	}
	if out.Funding == nil || *out.Funding != "Series B — $50M" {
		t.Errorf("Funding = %v, want %q", out.Funding, "Series B — $50M")
	}
	if out.GlassdoorRating == nil || *out.GlassdoorRating != 3.8 {
		t.Errorf("GlassdoorRating = %v, want 3.8", out.GlassdoorRating)
	}
	if out.FetchedAt != "2026-01-02T00:00:00Z" {
		t.Errorf("FetchedAt = %q, want the latest signal timestamp", out.FetchedAt)
	}
}

func TestRefresh_NoCompanyName(t *testing.T) {
	repo := &fakeRepo{job: sqlcgen.Job{Company: ""}}
	svc := companyintel.NewService(repo, companyintel.NewRegistry(), time.Millisecond)

	_, err := svc.Refresh(context.Background(), testJobID)
	if !errors.Is(err, companyintel.ErrNoCompany) {
		t.Errorf("err = %v, want ErrNoCompany", err)
	}
}

func TestRefresh_PersistsSuccessfulSignalsAndSkipsFailures(t *testing.T) {
	companyID := mustUUID(t, testCompanyID)
	repo := &fakeRepo{
		job:     sqlcgen.Job{Company: "Acme Corp"},
		company: sqlcgen.Company{ID: companyID, Name: "Acme Corp"},
	}
	registry := companyintel.NewRegistry(
		fakeScraper{kind: companyintel.KindFunding, domain: "crunchbase.com",
			result: &companyintel.SignalResult{Kind: companyintel.KindFunding, Value: "Series B", Source: "https://crunchbase.com/x"}},
		fakeScraper{kind: companyintel.KindGlassdoorRating, domain: "glassdoor.com",
			err: errors.New("blocked")},
	)
	svc := companyintel.NewService(repo, registry, time.Millisecond)

	// After Refresh, GetCompanySignals is called again to build the response —
	// the fake doesn't persist into `signals`, so seed it to reflect what the
	// fake UpsertCompanySignal call recorded.
	repo.signals = []sqlcgen.CompanySignal{
		{CompanyId: companyID, Kind: companyintel.KindFunding, Value: jsonValue(t, "Series B")},
	}

	out, err := svc.Refresh(context.Background(), testJobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("expected exactly 1 persisted signal (the successful one), got %d", len(repo.upserted))
	}
	if repo.upserted[0].Kind != companyintel.KindFunding {
		t.Errorf("persisted kind = %q, want %q", repo.upserted[0].Kind, companyintel.KindFunding)
	}
	if out.Funding == nil || *out.Funding != "Series B" {
		t.Errorf("Funding = %v, want %q", out.Funding, "Series B")
	}
	if out.Error != nil {
		t.Errorf("expected no top-level error on a partial success, got %q", *out.Error)
	}
}

func TestRefresh_AllSourcesFail_SetsTopLevelError(t *testing.T) {
	companyID := mustUUID(t, testCompanyID)
	repo := &fakeRepo{
		job:     sqlcgen.Job{Company: "Acme Corp"},
		company: sqlcgen.Company{ID: companyID, Name: "Acme Corp"},
		signals: []sqlcgen.CompanySignal{
			{CompanyId: companyID, Kind: companyintel.KindFunding, Value: jsonValue(t, "Series A (cached)")},
		},
	}
	registry := companyintel.NewRegistry(
		fakeScraper{kind: companyintel.KindFunding, domain: "crunchbase.com", err: errors.New("down")},
		fakeScraper{kind: companyintel.KindGlassdoorRating, domain: "glassdoor.com", err: errors.New("blocked")},
	)
	svc := companyintel.NewService(repo, registry, time.Millisecond)

	out, err := svc.Refresh(context.Background(), testJobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Error == nil {
		t.Fatal("expected a top-level error when every source fails")
	}
	// Previously-cached value must remain untouched.
	if out.Funding == nil || *out.Funding != "Series A (cached)" {
		t.Errorf("Funding = %v, want the previously-cached value to survive", out.Funding)
	}
	if len(repo.upserted) != 0 {
		t.Errorf("expected no signals persisted when every scraper fails, got %d", len(repo.upserted))
	}
}
