//go:build integration

package worker

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
)

func setupPersistTest(t *testing.T) (context.Context, *db.DB, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	// Own database per test (internal/dbtest): no shared tables, so no
	// truncation and no cross-package coordination. dbtest drops it on cleanup.
	testDB := dbtest.New(t)
	cleanup := cancel

	if err := testDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key: "adzuna", Kind: "api", Config: []byte(`{}`),
	}); err != nil {
		cleanup()
		t.Fatalf("upsert job source: %v", err)
	}

	return ctx, testDB, cleanup
}

func startRun(t *testing.T, ctx context.Context, testDB *db.DB) sqlcgen.SourceRun {
	t.Helper()
	source, err := testDB.Queries.GetJobSourceByKey(ctx, "adzuna")
	if err != nil {
		t.Fatalf("get job source: %v", err)
	}
	run, err := testDB.Queries.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{SourceId: source.ID})
	if err != nil {
		t.Fatalf("insert source run: %v", err)
	}
	return run
}

func testPostings(n int) []dto.NormalizedJob {
	out := make([]dto.NormalizedJob, 0, n)
	for i := range n {
		title := "Engineer " + strconv.Itoa(i)
		out = append(out, dto.NormalizedJob{
			SourceKey: "adzuna",
			Company:   "Acme",
			Title:     title,
			URL:       "https://example.test/" + strconv.Itoa(i),
			Raw:       map[string]any{},
		})
	}
	return out
}

func countJobs(t *testing.T, ctx context.Context, testDB *db.DB) int {
	t.Helper()
	var n int
	if err := testDB.Pool.QueryRow(ctx, `SELECT count(*) FROM "Job"`).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	return n
}

func seenCount(t *testing.T, ctx context.Context, testDB *db.DB, key string) int {
	t.Helper()
	var n int
	if err := testDB.Pool.QueryRow(ctx, `SELECT "seenCount" FROM "Job" WHERE "dedupeKey" = $1`, key).Scan(&n); err != nil {
		t.Fatalf("seen count: %v", err)
	}
	return n
}

// failingRepo passes everything through until the chosen statement, then
// fails — standing in for a mid-persist error such as a lost connection.
type failingRepo struct {
	domain.BatchRepository
	failOn string
}

var errInjected = errors.New("injected persist failure")

func (f *failingRepo) BulkInsertActivities(ctx context.Context, arg sqlcgen.BulkInsertActivitiesParams) ([]sqlcgen.BulkInsertActivitiesRow, error) {
	if f.failOn == "BulkInsertActivities" {
		return nil, errInjected
	}
	return f.BatchRepository.BulkInsertActivities(ctx, arg)
}

// countingRepo counts the batch statements a run issues.
type countingRepo struct {
	domain.BatchRepository
	statements int
}

func (c *countingRepo) GetJobsByDedupeKeys(ctx context.Context, keys []string) ([]sqlcgen.GetJobsByDedupeKeysRow, error) {
	c.statements++
	return c.BatchRepository.GetJobsByDedupeKeys(ctx, keys)
}

func (c *countingRepo) FindJobsByCompanies(ctx context.Context, arg sqlcgen.FindJobsByCompaniesParams) ([]sqlcgen.FindJobsByCompaniesRow, error) {
	c.statements++
	return c.BatchRepository.FindJobsByCompanies(ctx, arg)
}

func (c *countingRepo) BulkInsertJobs(ctx context.Context, arg sqlcgen.BulkInsertJobsParams) ([]sqlcgen.BulkInsertJobsRow, error) {
	c.statements++
	return c.BatchRepository.BulkInsertJobs(ctx, arg)
}

func (c *countingRepo) BulkRecordJobReposts(ctx context.Context, arg sqlcgen.BulkRecordJobRepostsParams) (int64, error) {
	c.statements++
	return c.BatchRepository.BulkRecordJobReposts(ctx, arg)
}

func (c *countingRepo) BulkMergeJobBoards(ctx context.Context, arg sqlcgen.BulkMergeJobBoardsParams) (int64, error) {
	c.statements++
	return c.BatchRepository.BulkMergeJobBoards(ctx, arg)
}

func (c *countingRepo) BulkInsertActivities(ctx context.Context, arg sqlcgen.BulkInsertActivitiesParams) ([]sqlcgen.BulkInsertActivitiesRow, error) {
	c.statements++
	return c.BatchRepository.BulkInsertActivities(ctx, arg)
}

// A failure anywhere in the persist phase must leave no posting visible and
// the run recorded as failed — a run's totals can never describe a storage
// phase that was rolled back.
func TestPersistRollsBackWholeRun(t *testing.T) {
	ctx, testDB, cleanup := setupPersistTest(t)
	defer cleanup()

	run := startRun(t, ctx, testDB)
	batch := NewPostingBatch(testPostings(20), pgtype.UUID{}, run.ID, false)

	err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		_, err := persistBatch(ctx, &failingRepo{BatchRepository: q, failOn: "BulkInsertActivities"}, batch, 500)
		return err
	})
	if !errors.Is(err, errInjected) {
		t.Fatalf("persist error = %v, want the injected failure", err)
	}

	if n := countJobs(t, ctx, testDB); n != 0 {
		t.Fatalf("%d postings visible after rollback, want 0", n)
	}

	msg := errInjected.Error()
	if err := testDB.Queries.FinishSourceRunError(ctx, sqlcgen.FinishSourceRunErrorParams{ID: run.ID, Error: &msg}); err != nil {
		t.Fatalf("finish run error: %v", err)
	}
	var ok *bool
	if err := testDB.Pool.QueryRow(ctx, `SELECT "ok" FROM "SourceRun" WHERE "id" = $1`, run.ID).Scan(&ok); err != nil {
		t.Fatalf("read run: %v", err)
	}
	if ok == nil || *ok {
		t.Fatalf("run ok = %v, want false", ok)
	}
}

// A retried run re-storing postings the failed attempt already stored must not
// inflate the sighting counts the ghost-job signal reads.
func TestPersistRetryCountsEachSightingOnce(t *testing.T) {
	ctx, testDB, cleanup := setupPersistTest(t)
	defer cleanup()

	run := startRun(t, ctx, testDB)
	jobs := testPostings(5)
	key := DedupeKey(jobs[0].Company, jobs[0].Title, jobs[0].URL)

	first := NewPostingBatch(jobs, pgtype.UUID{}, run.ID, false)
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		_, err := persistBatch(ctx, q, first, 500)
		return err
	}); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	after := seenCount(t, ctx, testDB, key)

	// Same run id: the second and third attempts are retries of one run.
	for range 2 {
		batch := NewPostingBatch(jobs, pgtype.UUID{}, run.ID, false)
		if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			_, err := persistBatch(ctx, q, batch, 500)
			return err
		}); err != nil {
			t.Fatalf("retry: %v", err)
		}
	}
	if got := seenCount(t, ctx, testDB, key); got != after+1 {
		t.Fatalf("seenCount = %d after retries, want %d — the guard counts a run once", got, after+1)
	}

	// A genuinely later run is a real re-sighting.
	next := startRun(t, ctx, testDB)
	batch := NewPostingBatch(jobs, pgtype.UUID{}, next.ID, false)
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		_, err := persistBatch(ctx, q, batch, 500)
		return err
	}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := seenCount(t, ctx, testDB, key); got != after+2 {
		t.Fatalf("seenCount = %d after a second run, want %d", got, after+2)
	}
}

// The design guarantees at most 6 statements per chunk. SC-002 sets the bar at
// 10, but asserting the looser number would let a regression from 6 to 9 pass
// silently.
func TestPersistStatementBudgetPerChunk(t *testing.T) {
	ctx, testDB, cleanup := setupPersistTest(t)
	defer cleanup()

	run := startRun(t, ctx, testDB)
	const count, chunkSize = 500, 100
	batch := NewPostingBatch(testPostings(count), pgtype.UUID{}, run.ID, false)

	counter := &countingRepo{}
	var result PersistResult
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		counter.BatchRepository = q
		var err error
		result, err = persistBatch(ctx, counter, batch, chunkSize)
		return err
	}); err != nil {
		t.Fatalf("persist: %v", err)
	}

	if len(result.Inserted) != count {
		t.Fatalf("Inserted = %d, want %d", len(result.Inserted), count)
	}
	if n := countJobs(t, ctx, testDB); n != count {
		t.Fatalf("stored %d postings, want %d", n, count)
	}
	chunks := count / chunkSize
	if max := 6 * chunks; counter.statements > max {
		t.Fatalf("%d statements for %d chunks, want at most %d", counter.statements, chunks, max)
	}
	if counter.statements != result.Statements {
		t.Fatalf("reported %d statements, issued %d", result.Statements, counter.statements)
	}
}

// Two runs storing the same posting concurrently: the unique constraint on
// dedupeKey arbitrates, exactly one row exists, and neither run fails.
func TestConcurrentRunsStoreOverlappingPostingOnce(t *testing.T) {
	ctx, testDB, cleanup := setupPersistTest(t)
	defer cleanup()

	runA := startRun(t, ctx, testDB)
	runB := startRun(t, ctx, testDB)
	shared := testPostings(1)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	inserted := make([]int, 2)
	for i, run := range []sqlcgen.SourceRun{runA, runB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch := NewPostingBatch(shared, pgtype.UUID{}, run.ID, false)
			errs[i] = testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
				result, err := persistBatch(ctx, q, batch, 500)
				inserted[i] = len(result.Inserted)
				return err
			})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("run %d failed: %v", i, err)
		}
	}
	if n := countJobs(t, ctx, testDB); n != 1 {
		t.Fatalf("%d rows for the shared posting, want 1", n)
	}
	if total := inserted[0] + inserted[1]; total != 1 {
		t.Fatalf("%d runs claimed the insert, want exactly 1", total)
	}
}
