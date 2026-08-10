package subscriptions_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/subscriptions"
)

type fakeRepo struct {
	subscriptions.Repository
	rows    []sqlcgen.Subscription
	listErr error
}

func (f *fakeRepo) ListSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error) {
	return f.rows, f.listErr
}

func (f *fakeRepo) CreateSubscription(ctx context.Context, arg sqlcgen.CreateSubscriptionParams) (sqlcgen.Subscription, error) {
	return sqlcgen.Subscription{SourceKey: arg.SourceKey, Name: arg.Name, Url: arg.Url, Enabled: arg.Enabled}, nil
}

type fakeSources struct{ subscriptions.SourceEnsurer }

func (f *fakeSources) GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error) {
	return sqlcgen.JobSource{Key: key}, nil
}

func TestListEmpty(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("len = %d, want 0", len(out))
	}
}

func TestListError(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{listErr: errors.New("db down")}, &fakeSources{})
	if _, err := svc.List(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreateIndeedSubscription_ValidURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "indeed", "https://www.indeed.com/jobs?q=golang&l=remote", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid indeed search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "indeed" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateIndeedSubscription_RejectsNonIndeedURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "indeed", "https://example.com/not-indeed", nil, true, "", ""); err == nil {
		t.Fatal("expected non-indeed url to be rejected")
	}
}

func TestCreateIndeedSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "indeed", "https://www.indeed.com/viewjob?jk=abc123", nil, true, "", ""); err == nil {
		t.Fatal("expected single job-posting url to be rejected, want a search results url")
	}
}

func TestCreateDouSubscription_UnaffectedByIndeedValidation(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "dou", "https://jobs.dou.ua/vacancies/?category=Golang", nil, true, "", ""); err != nil {
		t.Fatalf("expected non-indeed source to be unaffected by indeed-specific validation, got: %v", err)
	}
}

func TestCreateRemoteOKSubscription_ValidTagURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "remoteok", "https://remoteok.com/remote-golang-jobs", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid remoteok tag url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "remoteok" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateRemoteOKSubscription_ValidAPIRootURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "remoteok", "https://remoteok.com/api", nil, true, "", ""); err != nil {
		t.Fatalf("expected valid remoteok api root url to be accepted, got error: %v", err)
	}
}

func TestCreateRemoteOKSubscription_ValidIoDomain(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "remoteok", "https://remoteok.io/remote-golang-jobs", nil, true, "", ""); err != nil {
		t.Fatalf("expected valid remoteok.io url to be accepted, got error: %v", err)
	}
}

func TestCreateRemoteOKSubscription_RejectsNonRemoteOKURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "remoteok", "https://example.com/not-remoteok", nil, true, "", ""); err == nil {
		t.Fatal("expected non-remoteok url to be rejected")
	}
}

func TestCreateGlassdoorSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "glassdoor", "https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_KO0,14.htm", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid glassdoor search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "glassdoor" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateGlassdoorSubscription_RejectsNonGlassdoorURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "glassdoor", "https://www.indeed.com/jobs?q=golang", nil, true, "", ""); err == nil {
		t.Fatal("expected non-glassdoor url to be rejected")
	}
}

func TestCreateGlassdoorSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "glassdoor", "https://www.glassdoor.com/job-listing/senior-golang-developer-novatech-JV_KO0,24_KE25,41.htm?jl=123", nil, true, "", ""); err == nil {
		t.Fatal("expected single-job-posting glassdoor url to be rejected")
	}
}

func TestCreateJobLeadsSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "jobleads", "https://www.jobleads.com/job-search?q=golang", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid jobleads search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "jobleads" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateJobLeadsSubscription_RejectsNonJobLeadsURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobleads", "https://example.com/not-jobleads", nil, true, "", ""); err == nil {
		t.Fatal("expected non-jobleads url to be rejected")
	}
}

func TestCreateWellfoundSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "wellfound", "https://wellfound.com/role/r/golang-engineer", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid wellfound search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "wellfound" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateJobgetherSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "jobgether", "https://jobgether.com/jobs/search?technology=go&remote=true", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid jobgether search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "jobgether" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateHimalayasSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "himalayas", "https://himalayas.app/jobs?categories=Backend-Engineering", nil, true, "", "")
	if err != nil {
		t.Fatalf("expected valid himalayas search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "himalayas" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateWellfoundSubscription_ValidSubdomainURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://www.wellfound.com/role/r/golang-engineer", nil, true, "", ""); err != nil {
		t.Fatalf("expected valid wellfound.com subdomain url to be accepted, got error: %v", err)
	}
}

func TestCreateWellfoundSubscription_ValidLegacyAngelCoURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://angel.co/role/r/golang-engineer", nil, true, "", ""); err != nil {
		t.Fatalf("expected valid legacy angel.co url to be accepted, got error: %v", err)
	}
}

func TestCreateWellfoundSubscription_RejectsNonWellfoundURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://www.indeed.com/jobs?q=golang", nil, true, "", ""); err == nil {
		t.Fatal("expected non-wellfound url to be rejected")
	}
}

func TestCreateWellfoundSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://wellfound.com/jobs/1234567-senior-golang-engineer", nil, true, "", ""); err == nil {
		t.Fatal("expected single-job-posting wellfound url to be rejected")
	}
}

func TestCreateHimalayasSubscription_ValidSubdomainURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://www.himalayas.app/jobs/remote?categories=Backend-Engineering,Design", nil, true, "", ""); err != nil {
		t.Fatalf("expected valid himalayas subdomain url to be accepted, got error: %v", err)
	}
}

func TestCreateHimalayasSubscription_RejectsNonHimalayasURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://example.com/jobs?categories=Backend-Engineering", nil, true, "", ""); err == nil {
		t.Fatal("expected non-himalayas url to be rejected")
	}
}

func TestCreateHimalayasSubscription_RejectsWrongPath(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://himalayas.app/companies/acme?categories=Backend-Engineering", nil, true, "", ""); err == nil {
		t.Fatal("expected non-/jobs path himalayas url to be rejected")
	}
}

func TestCreateHimalayasSubscription_RejectsMissingCategories(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://himalayas.app/jobs", nil, true, "", ""); err == nil {
		t.Fatal("expected himalayas url missing 'categories' to be rejected")
	}
}

func TestCreateJobgetherSubscription_ValidSubdomainURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobgether", "https://www.jobgether.com/jobs/search?technology=go", nil, true, "", ""); err != nil {
		t.Fatalf("expected valid jobgether subdomain search url to be accepted, got error: %v", err)
	}
}

func TestCreateJobgetherSubscription_RejectsNonJobgetherURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobgether", "https://example.com/not-jobgether", nil, true, "", ""); err == nil {
		t.Fatal("expected non-jobgether url to be rejected")
	}
}

func TestCreateJobgetherSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobgether", "https://jobgether.com/jobs/senior-backend-engineer-go-waveform-labs-77213", nil, true, "", ""); err == nil {
		t.Fatal("expected single-job-posting jobgether url to be rejected")
	}
}

func TestValidateDjinniSubscriptionURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "basic-search URL with all filters accepted",
			url:     "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote",
			wantErr: false,
		},
		{
			name:    "basic-search URL with only search_type+primary_keyword accepted",
			url:     "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang",
			wantErr: false,
		},
		{
			name:    "basic-search URL with page=N already present accepted",
			url:     "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&page=4",
			wantErr: false,
		},
		{
			name:    "basic-search URL with unrecognized extra param accepted",
			url:     "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Go&foo=bar",
			wantErr: false,
		},
		{
			name:        "dashboard URL rejected",
			url:         "https://djinni.co/my/dashboard/subs/42/",
			wantErr:     true,
			errContains: "dashboard URLs are no longer supported",
		},
		{
			name:        "single-job-posting path without search_type rejected",
			url:         "https://djinni.co/jobs/12345",
			wantErr:     true,
			errContains: "dashboard URLs are no longer supported",
		},
		{
			name:        "missing search_type rejected",
			url:         "https://djinni.co/jobs/?primary_keyword=Golang",
			wantErr:     true,
			errContains: "dashboard URLs are no longer supported",
		},
		{
			name:        "non-djinni.co host rejected",
			url:         "https://example.com/jobs",
			wantErr:     true,
			errContains: "must be a djinni.co url",
		},
		{
			name:        "/companies path rejected",
			url:         "https://djinni.co/companies/some-company",
			wantErr:     true,
			errContains: "dashboard URLs are no longer supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
			_, err := svc.Create(context.Background(), "djinni", tt.url, nil, true, "", "")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// --- 041 manual-kind guards ---

type manualRepo struct {
	subscriptions.Repository
	rows       map[string]sqlcgen.Subscription
	ensured    []string
	jobCount   int64
	deleted    bool
	updateSeen bool
}

func newManualRepo(rows ...sqlcgen.Subscription) *manualRepo {
	r := &manualRepo{rows: map[string]sqlcgen.Subscription{}}
	for _, row := range rows {
		r.rows[dbutil.UUIDString(row.ID)] = row
	}
	return r
}

func (f *manualRepo) EnsureManualSubscription(ctx context.Context, arg sqlcgen.EnsureManualSubscriptionParams) (sqlcgen.Subscription, error) {
	for _, row := range f.rows {
		if row.SourceKey == arg.SourceKey && row.Kind == "manual" {
			f.ensured = append(f.ensured, arg.SourceKey)
			return row, nil
		}
	}
	row := sqlcgen.Subscription{
		ID:        pgtype.UUID{Bytes: [16]byte{byte(len(f.rows) + 1)}, Valid: true},
		SourceKey: arg.SourceKey,
		Name:      ptr("Manual"),
		Url:       "",
		Enabled:   true,
		Cron:      arg.Cron,
		Kind:      "manual",
	}
	f.rows[dbutil.UUIDString(row.ID)] = row
	f.ensured = append(f.ensured, arg.SourceKey)
	return row, nil
}

func (f *manualRepo) GetSubscription(ctx context.Context, id pgtype.UUID) (sqlcgen.Subscription, error) {
	row, ok := f.rows[dbutil.UUIDString(id)]
	if !ok {
		return sqlcgen.Subscription{}, errors.New("not found")
	}
	return row, nil
}

func (f *manualRepo) CountJobsForSubscription(ctx context.Context, id pgtype.UUID) (int64, error) {
	return f.jobCount, nil
}

func (f *manualRepo) DeleteSubscription(ctx context.Context, id pgtype.UUID) error {
	f.deleted = true
	return nil
}

func (f *manualRepo) UpdateSubscription(ctx context.Context, arg sqlcgen.UpdateSubscriptionParams) (sqlcgen.Subscription, error) {
	f.updateSeen = true
	return f.rows[dbutil.UUIDString(arg.ID)], nil
}

func ptr(s string) *string { return &s }

func manualRow() sqlcgen.Subscription {
	return sqlcgen.Subscription{
		ID:        pgtype.UUID{Bytes: [16]byte{42}, Valid: true},
		SourceKey: "djinni",
		Name:      ptr("Manual"),
		Enabled:   true,
		Cron:      "0 */6 * * *",
		Kind:      "manual",
	}
}

func manualRowID() string { return dbutil.UUIDString(manualRow().ID) }

func TestEnsureManualSubscription_CreatesOnFirstUseAndReusesAfter(t *testing.T) {
	repo := newManualRepo()
	svc := subscriptions.NewService(repo, &fakeSources{})

	first, err := svc.EnsureManualSubscription(context.Background(), "djinni")
	if err != nil {
		t.Fatalf("EnsureManualSubscription: %v", err)
	}
	second, err := svc.EnsureManualSubscription(context.Background(), "djinni")
	if err != nil {
		t.Fatalf("EnsureManualSubscription (second): %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("expected one manual subscription per source, got %v and %v", first.ID, second.ID)
	}
	if first.Kind != "manual" {
		t.Errorf("kind = %q, want manual", first.Kind)
	}
	if first.Url != "" {
		t.Errorf("expected a manual row to carry no url, got %q", first.Url)
	}
	if first.Name == nil || *first.Name != "Manual" {
		t.Errorf("expected the row to be named Manual, got %v", first.Name)
	}
}

func TestCreate_RejectsManualKind(t *testing.T) {
	svc := subscriptions.NewService(newManualRepo(), &fakeSources{})

	_, err := svc.Create(context.Background(), "djinni",
		"https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang", nil, true, "", "manual")

	if err == nil {
		t.Fatal("expected an explicit manual create to be rejected")
	}
	if !strings.Contains(err.Error(), "created implicitly") {
		t.Errorf("expected the refusal to say manual rows are implicit, got %v", err)
	}
}

func TestUpdate_ManualRowUrlAndCronAreImmutable(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   subscriptions.UpdateInput
	}{
		{"url", subscriptions.UpdateInput{URL: ptr("https://djinni.co/jobs/")}},
		{"cron", subscriptions.UpdateInput{Cron: ptr("0 * * * *")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newManualRepo(manualRow())
			svc := subscriptions.NewService(repo, &fakeSources{})

			if _, err := svc.Update(context.Background(), manualRowID(), tc.in); err == nil {
				t.Fatalf("expected %s to be immutable on a manual row", tc.name)
			}
			if repo.updateSeen {
				t.Errorf("expected the write to be refused before it reached the repository")
			}
		})
	}
}

func TestUpdate_ManualRowNameStaysEditable(t *testing.T) {
	repo := newManualRepo(manualRow())
	svc := subscriptions.NewService(repo, &fakeSources{})

	if _, err := svc.Update(context.Background(), manualRowID(), subscriptions.UpdateInput{Name: ptr("Hand-added")}); err != nil {
		t.Fatalf("expected renaming a manual row to be allowed, got %v", err)
	}
}

func TestDelete_ManualRowRefusedWhileVacanciesAttached(t *testing.T) {
	repo := newManualRepo(manualRow())
	repo.jobCount = 3
	svc := subscriptions.NewService(repo, &fakeSources{})

	err := svc.Delete(context.Background(), manualRowID())
	if err == nil {
		t.Fatal("expected a manual row with vacancies attached to refuse deletion")
	}
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Kind != apperr.KindConflict {
		t.Errorf("expected a conflict, got %v", err)
	}
	if repo.deleted {
		t.Error("expected nothing to be deleted")
	}
}

func TestDelete_ManualRowAllowedWhenEmpty(t *testing.T) {
	repo := newManualRepo(manualRow())
	svc := subscriptions.NewService(repo, &fakeSources{})

	if err := svc.Delete(context.Background(), manualRowID()); err != nil {
		t.Fatalf("expected an empty manual row to be deletable, got %v", err)
	}
	if !repo.deleted {
		t.Error("expected the row to be deleted")
	}
}

func TestUpdate_ManualRowCannotBeDisabledWhileVacanciesAttached(t *testing.T) {
	repo := newManualRepo(manualRow())
	repo.jobCount = 1
	svc := subscriptions.NewService(repo, &fakeSources{})

	disabled := false
	if _, err := svc.Update(context.Background(), manualRowID(), subscriptions.UpdateInput{Enabled: &disabled}); err == nil {
		t.Fatal("expected disabling a manual row with vacancies to be refused")
	}
}
