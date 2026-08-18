package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/application/ingest"
	"github.com/job-finder/api/internal/manualadd/application"
	"github.com/job-finder/api/internal/manualadd/domain"
	"github.com/nonamecat19/job-scraper/ports"
)

const postingURL = "https://djinni.co/jobs/123456-senior-go-engineer/"

type fakeRepo struct {
	domain.Repository

	runs      []sqlcgen.InsertSourceRunParams
	runErrors []string
	inserted  []sqlcgen.BulkInsertJobsParams
	knownKeys map[string]pgtype.UUID
	touched   int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{knownKeys: map[string]pgtype.UUID{}}
}

func (f *fakeRepo) InsertSourceRun(_ context.Context, arg sqlcgen.InsertSourceRunParams) (sqlcgen.SourceRun, error) {
	f.runs = append(f.runs, arg)
	return sqlcgen.SourceRun{ID: uuidOf(byte(len(f.runs)))}, nil
}

func (f *fakeRepo) FinishSourceRunError(_ context.Context, arg sqlcgen.FinishSourceRunErrorParams) error {
	if arg.Error != nil {
		f.runErrors = append(f.runErrors, *arg.Error)
	}
	return nil
}

func (f *fakeRepo) FinishSourceRunOk(context.Context, sqlcgen.FinishSourceRunOkParams) error {
	return nil
}

func (f *fakeRepo) TouchSubscriptionLastRun(context.Context, pgtype.UUID) error {
	f.touched++
	return nil
}

func (f *fakeRepo) GetJobsByDedupeKeys(_ context.Context, keys []string) ([]sqlcgen.GetJobsByDedupeKeysRow, error) {
	var out []sqlcgen.GetJobsByDedupeKeysRow
	for _, k := range keys {
		if id, ok := f.knownKeys[k]; ok {
			out = append(out, sqlcgen.GetJobsByDedupeKeysRow{ID: id, DedupeKey: k})
		}
	}
	return out, nil
}

func (f *fakeRepo) FindJobsByCompanies(context.Context, sqlcgen.FindJobsByCompaniesParams) ([]sqlcgen.FindJobsByCompaniesRow, error) {
	return nil, nil
}

func (f *fakeRepo) BulkInsertJobs(_ context.Context, arg sqlcgen.BulkInsertJobsParams) ([]sqlcgen.BulkInsertJobsRow, error) {
	f.inserted = append(f.inserted, arg)
	out := make([]sqlcgen.BulkInsertJobsRow, 0, len(arg.DedupeKeys))
	for i, key := range arg.DedupeKeys {
		out = append(out, sqlcgen.BulkInsertJobsRow{ID: uuidOf(byte(100 + i)), DedupeKey: key})
	}
	return out, nil
}

func (f *fakeRepo) BulkRecordJobReposts(context.Context, sqlcgen.BulkRecordJobRepostsParams) (int64, error) {
	return 0, nil
}

func (f *fakeRepo) BulkMergeJobBoards(context.Context, sqlcgen.BulkMergeJobBoardsParams) (int64, error) {
	return 0, nil
}

func (f *fakeRepo) BulkInsertActivities(context.Context, sqlcgen.BulkInsertActivitiesParams) ([]sqlcgen.BulkInsertActivitiesRow, error) {
	return nil, nil
}

func (f *fakeRepo) GetJobByDedupeKey(_ context.Context, key string) (pgtype.UUID, error) {
	if id, ok := f.knownKeys[key]; ok {
		return id, nil
	}
	return pgtype.UUID{}, pgx.ErrNoRows
}

type fakeSources struct{}

func (fakeSources) GetByKey(_ context.Context, key string) (sqlcgen.JobSource, error) {
	return sqlcgen.JobSource{ID: uuidOf(200), Key: key, Config: []byte(`{}`)}, nil
}

func (fakeSources) DecryptConfig([]byte) map[string]any { return map[string]any{} }

type fakeSubs struct{ ensured []string }

func (f *fakeSubs) EnsureManualSubscription(_ context.Context, sourceKey string) (sqlcgen.Subscription, error) {
	f.ensured = append(f.ensured, sourceKey)
	return sqlcgen.Subscription{ID: uuidOf(210), SourceKey: sourceKey, Kind: "manual"}, nil
}

type fakeJobs struct{}

func (fakeJobs) Get(_ context.Context, id string) (dto.JobDto, error) {
	return dto.JobDto{ID: id, Title: "Senior Go Engineer", Company: "NovaTech"}, nil
}

type fakeEnqueuer struct{ types []string }

func (f *fakeEnqueuer) EnqueueContext(_ context.Context, workType string, _ []byte) error {
	f.types = append(f.types, workType)
	return nil
}

type stubAdapter struct {
	key      string
	matches  func(string) bool
	claims   func(string) bool
	posting  dto.NormalizedJob
	readErr  error
	readCall int
}

func (a *stubAdapter) Key() string          { return a.key }
func (a *stubAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (a *stubAdapter) Search(context.Context, dto.SearchQuery, map[string]any) ([]dto.NormalizedJob, error) {
	return nil, nil
}

func (a *stubAdapter) HealthCheck(context.Context, map[string]any) (bool, error) { return true, nil }

func (a *stubAdapter) MatchesPostingURL(rawURL string) bool {
	if a.matches == nil {
		return false
	}
	return a.matches(rawURL)
}

func (a *stubAdapter) ClaimsHost(host string) bool {
	if a.claims == nil {
		return false
	}
	return a.claims(host)
}

func (a *stubAdapter) ReadPosting(context.Context, string, map[string]any) (dto.NormalizedJob, error) {
	a.readCall++
	if a.readErr != nil {
		return dto.NormalizedJob{}, a.readErr
	}
	return a.posting, nil
}

type blindAdapter struct{ key string }

func (a blindAdapter) Key() string          { return a.key }
func (a blindAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }
func (a blindAdapter) Search(context.Context, dto.SearchQuery, map[string]any) ([]dto.NormalizedJob, error) {
	return nil, nil
}
func (a blindAdapter) HealthCheck(context.Context, map[string]any) (bool, error) { return true, nil }

type fakeRegistry struct{ adapters []ports.JobSource }

func (r fakeRegistry) All() []ports.JobSource { return r.adapters }

func uuidOf(b byte) pgtype.UUID {
	id := pgtype.UUID{Valid: true}
	id.Bytes[0] = b
	return id
}

func completePosting() dto.NormalizedJob {
	return dto.NormalizedJob{
		SourceKey:   "djinni",
		Title:       "Senior Go Engineer",
		Company:     "NovaTech",
		Description: "Own the ledger service.",
		URL:         postingURL,
	}
}

type harness struct {
	svc     *application.Service
	repo    *fakeRepo
	subs    *fakeSubs
	enq     *fakeEnqueuer
	adapter *stubAdapter
}

func newHarness(t *testing.T, adapter *stubAdapter, opts ...application.Option) *harness {
	t.Helper()
	repo := newFakeRepo()
	subs := &fakeSubs{}
	enq := &fakeEnqueuer{}
	adapters := []ports.JobSource{blindAdapter{key: "adzuna"}}
	if adapter != nil {
		adapters = append(adapters, adapter)
	}
	svc := application.NewService(repo, fakeSources{}, subs, fakeRegistry{adapters}, fakeJobs{}, enq, opts...)
	return &harness{svc: svc, repo: repo, subs: subs, enq: enq, adapter: adapter}
}

func djinniStub() *stubAdapter {
	return &stubAdapter{
		key:     "djinni",
		matches: func(u string) bool { return strings.Contains(u, "djinni.co/jobs/123456-") },
		claims:  func(host string) bool { return host == "djinni.co" },
		posting: completePosting(),
	}
}

func TestAdd_CreatesVacancy(t *testing.T) {
	h := newHarness(t, djinniStub())

	result, err := h.svc.Add(context.Background(), postingURL)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if result.Outcome != domain.OutcomeCreated {
		t.Fatalf("outcome = %q, want created (%+v)", result.Outcome, result)
	}
	if result.Job == nil {
		t.Fatal("expected the created vacancy to come back with the response")
	}
	if len(h.repo.runs) != 1 {
		t.Fatalf("expected exactly one run per attempt, got %d", len(h.repo.runs))
	}
	run := h.repo.runs[0]
	if run.Trigger != "manual" {
		t.Errorf("run trigger = %q, want manual", run.Trigger)
	}
	if !run.SubscriptionId.Valid {
		t.Error("expected the run to be attributed to the manual subscription")
	}
	if len(h.subs.ensured) != 1 || h.subs.ensured[0] != "djinni" {
		t.Errorf("expected the manual subscription for djinni to be ensured, got %v", h.subs.ensured)
	}
	if h.repo.touched != 1 {
		t.Errorf("expected the manual subscription's lastRunAt to be touched once, got %d", h.repo.touched)
	}
}

func TestAdd_CreatedEnqueuesMatchAndGhostScore(t *testing.T) {
	h := newHarness(t, djinniStub())

	if _, err := h.svc.Add(context.Background(), postingURL); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if len(h.enq.types) != 2 {
		t.Fatalf("expected match and ghost-score to be enqueued, got %v", h.enq.types)
	}
}

func TestAdd_DuplicateReturnsExistingVacancyAndIsNotAnError(t *testing.T) {
	h := newHarness(t, djinniStub())

	posting := completePosting()
	h.repo.knownKeys[dedupeKeyFor(posting)] = uuidOf(50)

	result, err := h.svc.Add(context.Background(), postingURL)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if result.Outcome != domain.OutcomeDuplicate {
		t.Fatalf("outcome = %q, want duplicate", result.Outcome)
	}
	if result.Job == nil {
		t.Fatal("expected the existing vacancy so the client can navigate to it")
	}
	if len(h.repo.inserted) != 0 {
		t.Error("expected no insert for a duplicate")
	}
	if len(h.enq.types) != 0 {
		t.Errorf("expected no downstream work for a duplicate, got %v", h.enq.types)
	}
	if len(h.repo.runs) != 1 {
		t.Errorf("expected a duplicate to still write exactly one run, got %d", len(h.repo.runs))
	}
}

func TestAdd_FailureTaxonomy(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		adapter    *stubAdapter
		wantKind   domain.FailureKind
		wantRun    bool
		wantReader bool
	}{
		{
			name:     "not http or https",
			url:      "ftp://djinni.co/jobs/1-a/",
			adapter:  djinniStub(),
			wantKind: domain.KindInvalidURL,
		},
		{
			name:     "unparseable",
			url:      "://///",
			adapter:  djinniStub(),
			wantKind: domain.KindInvalidURL,
		},
		{
			name:     "known host, search page",
			url:      "https://djinni.co/jobs/?search_type=basic-search",
			adapter:  djinniStub(),
			wantKind: domain.KindNotAPosting,
		},
		{
			name:     "unknown host",
			url:      "https://careers.example.com/jobs/42",
			adapter:  djinniStub(),
			wantKind: domain.KindNoReader,
		},
		{
			name: "unreachable",
			url:  postingURL,
			adapter: func() *stubAdapter {
				a := djinniStub()
				a.readErr = errors.New("dial tcp: connection refused")
				return a
			}(),
			wantKind:   domain.KindUnreachable,
			wantRun:    true,
			wantReader: true,
		},
		{
			name: "blocked",
			url:  postingURL,
			adapter: func() *stubAdapter {
				a := djinniStub()
				a.readErr = errors.New("djinni: the posting is behind a login wall")
				return a
			}(),
			wantKind:   domain.KindBlocked,
			wantRun:    true,
			wantReader: true,
		},
		{
			name: "timed out",
			url:  postingURL,
			adapter: func() *stubAdapter {
				a := djinniStub()
				a.readErr = context.DeadlineExceeded
				return a
			}(),
			wantKind:   domain.KindTimedOut,
			wantRun:    true,
			wantReader: true,
		},
		{
			name: "read but incomplete",
			url:  postingURL,
			adapter: func() *stubAdapter {
				a := djinniStub()
				a.posting = dto.NormalizedJob{SourceKey: "djinni", Title: "Senior Go Engineer", URL: postingURL}
				return a
			}(),
			wantKind:   domain.KindIncomplete,
			wantRun:    true,
			wantReader: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.adapter)

			result, err := h.svc.Add(context.Background(), tt.url)
			if err != nil {
				t.Fatalf("Add: %v", err)
			}

			if result.Outcome != domain.OutcomeFailed {
				t.Fatalf("outcome = %q, want failed", result.Outcome)
			}
			if result.Kind != tt.wantKind {
				t.Fatalf("kind = %q, want %q", result.Kind, tt.wantKind)
			}
			if result.Reason == "" {
				t.Error("expected an operator-facing reason")
			}
			if len(h.repo.inserted) != 0 {
				t.Error("a failed attempt must leave no vacancy behind")
			}
			if len(h.enq.types) != 0 {
				t.Errorf("a failed attempt must enqueue nothing, got %v", h.enq.types)
			}
			if tt.wantRun {
				if len(h.repo.runs) != 1 {
					t.Errorf("expected the attempt to be recorded as one run, got %d", len(h.repo.runs))
				}
				if len(h.repo.runErrors) != 1 {
					t.Errorf("expected the run to be closed with the failure reason, got %v", h.repo.runErrors)
				}
			}
			if !tt.wantReader && tt.adapter.readCall != 0 {
				t.Error("expected no network read for a URL that never resolved")
			}
		})
	}
}

func TestAdd_NoReaderAndIncompleteCarryADraftOnceFillInExists(t *testing.T) {
	incomplete := djinniStub()
	incomplete.posting = dto.NormalizedJob{
		SourceKey: "djinni", Title: "Senior Go Engineer", Company: "NovaTech", URL: postingURL, Remote: true,
	}
	h := newHarness(t, incomplete, application.WithFillIn())

	result, err := h.svc.Add(context.Background(), postingURL)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if result.Outcome != domain.OutcomeNeedsFillIn {
		t.Fatalf("outcome = %q, want needs_fill_in", result.Outcome)
	}
	if result.Draft == nil {
		t.Fatal("expected a draft so the operator completes rather than retypes")
	}
	if result.Draft.Title == nil || *result.Draft.Title != "Senior Go Engineer" {
		t.Errorf("draft lost the extracted title: %+v", result.Draft)
	}
	if result.Draft.URL != postingURL {
		t.Errorf("draft url = %q, want the submitted URL", result.Draft.URL)
	}

	unknownHost := newHarness(t, djinniStub(), application.WithFillIn())
	noReader, err := unknownHost.svc.Add(context.Background(), "https://careers.example.com/jobs/42")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if noReader.Outcome != domain.OutcomeNeedsFillIn || noReader.Kind != domain.KindNoReader {
		t.Fatalf("outcome = %q kind = %q, want needs_fill_in/no_reader", noReader.Outcome, noReader.Kind)
	}
	if noReader.Draft == nil || noReader.Draft.URL != "https://careers.example.com/jobs/42" {
		t.Errorf("expected the draft to retain the submitted URL, got %+v", noReader.Draft)
	}
}

func TestAdd_ValidatesURLBeforeAnyNetworkCall(t *testing.T) {
	adapter := djinniStub()
	h := newHarness(t, adapter)

	if _, err := h.svc.Add(context.Background(), "not-a-url"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if adapter.readCall != 0 {
		t.Error("expected an invalid URL to be rejected before any fetch")
	}
	if len(h.repo.runs) != 0 {
		t.Error("expected no run for a URL that never reached a source")
	}
}

func dedupeKeyFor(j dto.NormalizedJob) string {
	return ingest.DedupeKey(j.Company, j.Title, j.URL)
}
