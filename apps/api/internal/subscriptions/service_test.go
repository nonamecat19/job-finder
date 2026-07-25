package subscriptions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
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
	out, err := svc.Create(context.Background(), "indeed", "https://www.indeed.com/jobs?q=golang&l=remote", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid indeed search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "indeed" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateIndeedSubscription_RejectsNonIndeedURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "indeed", "https://example.com/not-indeed", nil, true, ""); err == nil {
		t.Fatal("expected non-indeed url to be rejected")
	}
}

func TestCreateIndeedSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "indeed", "https://www.indeed.com/viewjob?jk=abc123", nil, true, ""); err == nil {
		t.Fatal("expected single job-posting url to be rejected, want a search results url")
	}
}

func TestCreateDouSubscription_UnaffectedByIndeedValidation(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "dou", "https://jobs.dou.ua/vacancies/?category=Golang", nil, true, ""); err != nil {
		t.Fatalf("expected non-indeed source to be unaffected by indeed-specific validation, got: %v", err)
	}
}

func TestCreateRemoteOKSubscription_ValidTagURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "remoteok", "https://remoteok.com/remote-golang-jobs", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid remoteok tag url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "remoteok" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateRemoteOKSubscription_ValidAPIRootURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "remoteok", "https://remoteok.com/api", nil, true, ""); err != nil {
		t.Fatalf("expected valid remoteok api root url to be accepted, got error: %v", err)
	}
}

func TestCreateRemoteOKSubscription_ValidIoDomain(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "remoteok", "https://remoteok.io/remote-golang-jobs", nil, true, ""); err != nil {
		t.Fatalf("expected valid remoteok.io url to be accepted, got error: %v", err)
	}
}

func TestCreateRemoteOKSubscription_RejectsNonRemoteOKURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "remoteok", "https://example.com/not-remoteok", nil, true, ""); err == nil {
		t.Fatal("expected non-remoteok url to be rejected")
	}
}

func TestCreateGlassdoorSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "glassdoor", "https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_KO0,14.htm", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid glassdoor search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "glassdoor" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateGlassdoorSubscription_RejectsNonGlassdoorURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "glassdoor", "https://www.indeed.com/jobs?q=golang", nil, true, ""); err == nil {
		t.Fatal("expected non-glassdoor url to be rejected")
	}
}

func TestCreateGlassdoorSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "glassdoor", "https://www.glassdoor.com/job-listing/senior-golang-developer-novatech-JV_KO0,24_KE25,41.htm?jl=123", nil, true, ""); err == nil {
		t.Fatal("expected single-job-posting glassdoor url to be rejected")
	}
}

func TestCreateJobLeadsSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "jobleads", "https://www.jobleads.com/job-search?q=golang", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid jobleads search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "jobleads" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateJobLeadsSubscription_RejectsNonJobLeadsURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobleads", "https://example.com/not-jobleads", nil, true, ""); err == nil {
		t.Fatal("expected non-jobleads url to be rejected")
	}
}

func TestCreateWellfoundSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "wellfound", "https://wellfound.com/role/r/golang-engineer", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid wellfound search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "wellfound" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateJobgetherSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "jobgether", "https://jobgether.com/jobs/search?technology=go&remote=true", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid jobgether search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "jobgether" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateHimalayasSubscription_ValidSearchURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	out, err := svc.Create(context.Background(), "himalayas", "https://himalayas.app/jobs?categories=Backend-Engineering", nil, true, "")
	if err != nil {
		t.Fatalf("expected valid himalayas search url to be accepted, got error: %v", err)
	}
	if out.SourceKey != "himalayas" {
		t.Errorf("sourceKey: got %q", out.SourceKey)
	}
}

func TestCreateWellfoundSubscription_ValidSubdomainURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://www.wellfound.com/role/r/golang-engineer", nil, true, ""); err != nil {
		t.Fatalf("expected valid wellfound.com subdomain url to be accepted, got error: %v", err)
	}
}

func TestCreateWellfoundSubscription_ValidLegacyAngelCoURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://angel.co/role/r/golang-engineer", nil, true, ""); err != nil {
		t.Fatalf("expected valid legacy angel.co url to be accepted, got error: %v", err)
	}
}

func TestCreateWellfoundSubscription_RejectsNonWellfoundURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://www.indeed.com/jobs?q=golang", nil, true, ""); err == nil {
		t.Fatal("expected non-wellfound url to be rejected")
	}
}

func TestCreateWellfoundSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "wellfound", "https://wellfound.com/jobs/1234567-senior-golang-engineer", nil, true, ""); err == nil {
		t.Fatal("expected single-job-posting wellfound url to be rejected")
	}
}

func TestCreateHimalayasSubscription_ValidSubdomainURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://www.himalayas.app/jobs/remote?categories=Backend-Engineering,Design", nil, true, ""); err != nil {
		t.Fatalf("expected valid himalayas subdomain url to be accepted, got error: %v", err)
	}
}

func TestCreateHimalayasSubscription_RejectsNonHimalayasURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://example.com/jobs?categories=Backend-Engineering", nil, true, ""); err == nil {
		t.Fatal("expected non-himalayas url to be rejected")
	}
}

func TestCreateHimalayasSubscription_RejectsWrongPath(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://himalayas.app/companies/acme?categories=Backend-Engineering", nil, true, ""); err == nil {
		t.Fatal("expected non-/jobs path himalayas url to be rejected")
	}
}

func TestCreateHimalayasSubscription_RejectsMissingCategories(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "himalayas", "https://himalayas.app/jobs", nil, true, ""); err == nil {
		t.Fatal("expected himalayas url missing 'categories' to be rejected")
	}
}

func TestCreateJobgetherSubscription_ValidSubdomainURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobgether", "https://www.jobgether.com/jobs/search?technology=go", nil, true, ""); err != nil {
		t.Fatalf("expected valid jobgether subdomain search url to be accepted, got error: %v", err)
	}
}

func TestCreateJobgetherSubscription_RejectsNonJobgetherURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobgether", "https://example.com/not-jobgether", nil, true, ""); err == nil {
		t.Fatal("expected non-jobgether url to be rejected")
	}
}

func TestCreateJobgetherSubscription_RejectsSingleJobPostingURL(t *testing.T) {
	svc := subscriptions.NewService(&fakeRepo{}, &fakeSources{})
	if _, err := svc.Create(context.Background(), "jobgether", "https://jobgether.com/jobs/senior-backend-engineer-go-waveform-labs-77213", nil, true, ""); err == nil {
		t.Fatal("expected single-job-posting jobgether url to be rejected")
	}
}
