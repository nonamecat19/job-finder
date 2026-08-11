//go:build integration

package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobs"
	"github.com/job-finder/api/internal/jobsources/application/ingest"
	"github.com/job-finder/api/internal/manualadd/application"
	"github.com/job-finder/api/internal/manualadd/domain"
	"github.com/job-finder/api/internal/subscriptions"
	jsadapter "github.com/nonamecat19/jobscraper/adapter"
	"github.com/nonamecat19/jobscraper/ports"
)

func setupManualAdd(t *testing.T) (context.Context, *db.DB, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	testDB := dbtest.New(t)

	if err := testDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key: "djinni", Kind: "scrape", Config: []byte(`{}`),
	}); err != nil {
		cancel()
		t.Fatalf("upsert job source: %v", err)
	}
	return ctx, testDB, cancel
}

// realSourceProvider is the jobsources service's two-method surface, satisfied
// directly against the test database rather than through the full service.
type realSourceProvider struct{ q *sqlcgen.Queries }

func (p realSourceProvider) GetByKey(ctx context.Context, key string) (sqlcgen.JobSource, error) {
	return p.q.GetJobSourceByKey(ctx, key)
}

func (p realSourceProvider) DecryptConfig([]byte) map[string]any { return map[string]any{} }

func newManualService(t *testing.T, testDB *db.DB, adapter ports.JobSource) *application.Service {
	t.Helper()
	subsSvc := subscriptions.NewService(testDB.Queries, realSourceProvider{testDB.Queries})
	jobsSvc := jobs.NewService(testDB.Queries, nil, 0)
	return application.NewService(
		testDB.Queries,
		realSourceProvider{testDB.Queries},
		subsSvc,
		jsadapter.MustRegistry(adapter),
		jobsSvc,
		nil,
		application.WithTxRunner(testDB),
	)
}

const integrationPostingURL = "https://djinni.co/jobs/999001-senior-go-engineer/"

func integrationPosting() dto.NormalizedJob {
	return dto.NormalizedJob{
		SourceKey:   "djinni",
		Title:       "Senior Go Engineer",
		Company:     "NovaTech",
		Description: "Own the ledger service.",
		URL:         integrationPostingURL,
		Raw:         map[string]any{},
	}
}

func countJobsWithKey(t *testing.T, ctx context.Context, testDB *db.DB, key string) int {
	t.Helper()
	var n int
	if err := testDB.Pool.QueryRow(ctx, `SELECT count(*) FROM "Job" WHERE "dedupeKey" = $1`, key).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	return n
}

// A manual add and a crawl of the same posting must collapse to one vacancy,
// because both go through the same dedupe key and the same insert (D5).
func TestIntegration_ManualAddAndCrawlOfTheSamePostingYieldOneVacancy(t *testing.T) {
	ctx, testDB, cleanup := setupManualAdd(t)
	defer cleanup()

	posting := integrationPosting()
	key := ingest.DedupeKey(posting.Company, posting.Title, posting.URL)

	svc := newManualService(t, testDB, &stubAdapter{
		key:     "djinni",
		matches: func(string) bool { return true },
		posting: posting,
	})

	result, err := svc.Add(ctx, integrationPostingURL)
	if err != nil {
		t.Fatalf("manual add: %v", err)
	}
	if result.Outcome != domain.OutcomeCreated {
		t.Fatalf("outcome = %q, want created", result.Outcome)
	}

	// The crawl path, with the same posting, through the same ingest package.
	source, err := testDB.Queries.GetJobSourceByKey(ctx, "djinni")
	if err != nil {
		t.Fatalf("get job source: %v", err)
	}
	run, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId: source.ID, Trigger: "scheduled",
	})
	if err != nil {
		t.Fatalf("insert crawl run: %v", err)
	}
	crawlBatch := ingest.NewPostingBatch([]dto.NormalizedJob{posting}, run.SubscriptionId, run.ID, false)
	crawled, err := ingest.PersistBatch(ctx, testDB.Queries, crawlBatch, ingest.DefaultChunkSize)
	if err != nil {
		t.Fatalf("crawl persist: %v", err)
	}
	if len(crawled.Inserted) != 0 {
		t.Errorf("expected the crawl to recognise the manually added vacancy, got %d inserts", len(crawled.Inserted))
	}

	if got := countJobsWithKey(t, ctx, testDB, key); got != 1 {
		t.Fatalf("vacancies with the shared dedupe key = %d, want 1", got)
	}
}

// FR-008: two simultaneous submissions of the same URL yield one vacancy. The
// loser of the race must report duplicate, not an error.
func TestIntegration_ConcurrentAddsOfTheSameURLYieldOneVacancy(t *testing.T) {
	ctx, testDB, cleanup := setupManualAdd(t)
	defer cleanup()

	posting := integrationPosting()
	key := ingest.DedupeKey(posting.Company, posting.Title, posting.URL)

	newAdapter := func() ports.JobSource {
		return &stubAdapter{key: "djinni", matches: func(string) bool { return true }, posting: posting}
	}

	var wg sync.WaitGroup
	outcomes := make([]domain.Outcome, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc := newManualService(t, testDB, newAdapter())
			<-start
			result, err := svc.Add(ctx, integrationPostingURL)
			outcomes[i], errs[i] = result.Outcome, err
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("add %d failed: %v", i, err)
		}
	}
	created := 0
	for _, outcome := range outcomes {
		switch outcome {
		case domain.OutcomeCreated:
			created++
		case domain.OutcomeDuplicate:
		default:
			t.Fatalf("unexpected outcome %q — a lost race is a duplicate, not a failure", outcome)
		}
	}
	if created != 1 {
		t.Errorf("created outcomes = %d, want exactly 1", created)
	}
	if got := countJobsWithKey(t, ctx, testDB, key); got != 1 {
		t.Fatalf("vacancies = %d, want 1", got)
	}
}

// FR-017g: three failed manual adds must leave the source healthy, because the
// health query excludes runs triggered by hand.
func TestIntegration_FailedManualAddsLeaveTheSourceHealthy(t *testing.T) {
	ctx, testDB, cleanup := setupManualAdd(t)
	defer cleanup()

	svc := newManualService(t, testDB, &stubAdapter{
		key:     "djinni",
		matches: func(string) bool { return true },
		readErr: context.DeadlineExceeded,
	})

	for range 3 {
		result, err := svc.Add(ctx, integrationPostingURL)
		if err != nil {
			t.Fatalf("manual add: %v", err)
		}
		if result.Outcome != domain.OutcomeFailed {
			t.Fatalf("outcome = %q, want failed", result.Outcome)
		}
	}

	source, err := testDB.Queries.GetJobSourceByKey(ctx, "djinni")
	if err != nil {
		t.Fatalf("get job source: %v", err)
	}
	recent, err := testDB.Queries.RecentSourceRunsForSource(ctx, sqlcgen.RecentSourceRunsForSourceParams{
		SourceId: source.ID, Limit: 3,
	})
	if err != nil {
		t.Fatalf("recent source runs: %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("expected manual runs to be invisible to health accounting, got %d", len(recent))
	}

	var n int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT count(*) FROM "SourceRun" WHERE "sourceId" = $1 AND "trigger" = 'manual'`, source.ID).Scan(&n); err != nil {
		t.Fatalf("count manual runs: %v", err)
	}
	if n != 3 {
		t.Errorf("manual runs recorded = %d, want 3 — every attempt writes exactly one run", n)
	}

	if got := countJobsWithKey(t, ctx, testDB, ingest.DedupeKey("NovaTech", "Senior Go Engineer", integrationPostingURL)); got != 0 {
		t.Errorf("expected failed attempts to leave no vacancy behind, got %d", got)
	}
}
