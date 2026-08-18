package worker_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
	"github.com/job-finder/api/internal/queue"
)

type schedulerFakeRepo struct {
	domain.SearchRepository
	search      sqlcgen.SavedSearch
	claimErr    error
	claimedWith pgtype.Timestamp
	claimCalls  int
	runStarted  bool

	unmatched       []sqlcgen.ListJobsMissingMatchRow
	reconcileWindow sqlcgen.ListJobsMissingMatchParams

	subs          []sqlcgen.Subscription
	subClaimErr   error
	subClaims     int
	subRunStarted bool
}

func (f *schedulerFakeRepo) ListEnabledSavedSearches(ctx context.Context) ([]sqlcgen.SavedSearch, error) {
	return []sqlcgen.SavedSearch{f.search}, nil
}

func (f *schedulerFakeRepo) ClaimSavedSearchRun(ctx context.Context, arg sqlcgen.ClaimSavedSearchRunParams) (pgtype.UUID, error) {
	f.claimCalls++
	f.claimedWith = arg.ExpectedLastRunAt
	if f.claimErr != nil {
		return pgtype.UUID{}, f.claimErr
	}
	return arg.ID, nil
}

func (f *schedulerFakeRepo) GetSavedSearch(ctx context.Context, id pgtype.UUID) (sqlcgen.SavedSearch, error) {
	f.runStarted = true
	return f.search, pgx.ErrNoRows
}

func (f *schedulerFakeRepo) DeleteActivityRunsBefore(ctx context.Context, createdat pgtype.Timestamp) error {
	return nil
}

func (f *schedulerFakeRepo) ListJobsMissingMatch(ctx context.Context, arg sqlcgen.ListJobsMissingMatchParams) ([]sqlcgen.ListJobsMissingMatchRow, error) {
	f.reconcileWindow = arg
	return f.unmatched, nil
}

func (f *schedulerFakeRepo) InsertActivityRun(ctx context.Context, arg sqlcgen.InsertActivityRunParams) (sqlcgen.ActivityRun, error) {
	return sqlcgen.ActivityRun{}, nil
}

func (f *schedulerFakeRepo) ListEnabledSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error) {
	return f.subs, nil
}

func (f *schedulerFakeRepo) ClaimSubscriptionRun(ctx context.Context, arg sqlcgen.ClaimSubscriptionRunParams) (pgtype.UUID, error) {
	f.subClaims++
	if f.subClaimErr != nil {
		return pgtype.UUID{}, f.subClaimErr
	}
	return arg.ID, nil
}

func (f *schedulerFakeRepo) GetSubscription(ctx context.Context, id pgtype.UUID) (sqlcgen.Subscription, error) {
	f.subRunStarted = true
	if len(f.subs) == 0 {
		return sqlcgen.Subscription{}, pgx.ErrNoRows
	}
	sub := f.subs[0]
	sub.Enabled = false
	return sub, nil
}

func dueSearch() sqlcgen.SavedSearch {
	return sqlcgen.SavedSearch{
		ID:        pgtype.UUID{Bytes: [16]byte{9}, Valid: true},
		Name:      "go roles",
		Cron:      "0 */6 * * *",
		Enabled:   true,
		LastRunAt: pgtype.Timestamp{Time: time.Now().Add(-24 * time.Hour), Valid: true},
	}
}

func TestTick_ClaimsBeforeRunning(t *testing.T) {
	repo := &schedulerFakeRepo{search: dueSearch()}
	s := worker.NewScheduler(repo, application.NewSearchService(repo, nil, nil, &fakeEnqueuer{}), nil, nil)

	s.Tick(context.Background())

	if repo.claimCalls != 1 {
		t.Fatalf("expected exactly one claim, got %d", repo.claimCalls)
	}
	if !repo.claimedWith.Valid || !repo.claimedWith.Time.Equal(repo.search.LastRunAt.Time) {
		t.Errorf("expected the claim to CAS against the observed lastRunAt %v, got %v",
			repo.search.LastRunAt.Time, repo.claimedWith)
	}
	if !repo.runStarted {
		t.Error("expected a won claim to proceed into RunSearch")
	}
}

func TestTick_LostClaimSkipsRun(t *testing.T) {
	repo := &schedulerFakeRepo{search: dueSearch(), claimErr: pgx.ErrNoRows}
	s := worker.NewScheduler(repo, application.NewSearchService(repo, nil, nil, &fakeEnqueuer{}), nil, nil)

	s.Tick(context.Background())

	if repo.runStarted {
		t.Error("expected a lost claim to skip the run entirely")
	}
}

func TestTick_ReconcilesJobsWithNoMatchResult(t *testing.T) {
	repo := &schedulerFakeRepo{
		search:   dueSearch(),
		claimErr: pgx.ErrNoRows,
		unmatched: []sqlcgen.ListJobsMissingMatchRow{
			{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, Company: "Acme", Title: "Go Developer"},
			{ID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, Company: "Globex", Title: "Backend Engineer"},
		},
	}
	enq := &countingEnqueuer{}
	s := worker.NewScheduler(repo, application.NewSearchService(repo, nil, nil, enq), nil, nil)

	s.Tick(context.Background())

	if enq.count != 2 {
		t.Fatalf("expected a match task per unmatched job, got %d", enq.count)
	}
	for _, task := range enq.types {
		if task != queue.TypeMatch {
			t.Errorf("expected only match tasks from the sweep, got %q", task)
		}
	}

	w := repo.reconcileWindow
	if !w.OlderThan.Valid || !w.NewerThan.Valid {
		t.Fatalf("expected the sweep to be bounded on both sides, got %+v", w)
	}
	if !w.NewerThan.Time.Before(w.OlderThan.Time) {
		t.Errorf("expected newer_than < older_than, got newer=%v older=%v", w.NewerThan.Time, w.OlderThan.Time)
	}
	if w.Limit <= 0 {
		t.Errorf("expected a batch cap so a backlog drains over several ticks, got %d", w.Limit)
	}
}

type countingEnqueuer struct {
	application.Enqueuer
	count int
	types []string
}

func (e *countingEnqueuer) EnqueueContext(ctx context.Context, workType string, payload []byte) error {
	e.count++
	e.types = append(e.types, workType)
	return nil
}

func dueSubscription() sqlcgen.Subscription {
	return sqlcgen.Subscription{
		ID:        pgtype.UUID{Bytes: [16]byte{7}, Valid: true},
		SourceKey: "djinni",
		Url:       "https://djinni.co/jobs/?primary_keyword=Golang",
		Enabled:   true,
		Cron:      "0 */6 * * *",
		LastRunAt: pgtype.Timestamp{Time: time.Now().Add(-24 * time.Hour), Valid: true},
	}
}

func TestTick_RunsDueSubscriptions(t *testing.T) {
	repo := &schedulerFakeRepo{
		search:   dueSearch(),
		claimErr: pgx.ErrNoRows,
		subs:     []sqlcgen.Subscription{dueSubscription()},
	}
	s := worker.NewScheduler(repo, application.NewSearchService(repo, nil, nil, &countingEnqueuer{}), nil, nil)

	s.Tick(context.Background())

	if repo.subClaims != 1 {
		t.Fatalf("expected exactly one subscription claim, got %d", repo.subClaims)
	}
	if !repo.subRunStarted {
		t.Error("expected a won claim to proceed into RunSubscription")
	}
}

func TestTick_SkipsSubscriptionNotYetDue(t *testing.T) {
	sub := dueSubscription()
	sub.LastRunAt = pgtype.Timestamp{Time: time.Now(), Valid: true}
	repo := &schedulerFakeRepo{search: dueSearch(), claimErr: pgx.ErrNoRows, subs: []sqlcgen.Subscription{sub}}
	s := worker.NewScheduler(repo, application.NewSearchService(repo, nil, nil, &countingEnqueuer{}), nil, nil)

	s.Tick(context.Background())

	if repo.subClaims != 0 {
		t.Errorf("expected a subscription just run not to be claimed again, got %d claims", repo.subClaims)
	}
}

func TestTick_LostSubscriptionClaimSkipsRun(t *testing.T) {
	repo := &schedulerFakeRepo{
		search:      dueSearch(),
		claimErr:    pgx.ErrNoRows,
		subs:        []sqlcgen.Subscription{dueSubscription()},
		subClaimErr: pgx.ErrNoRows,
	}
	s := worker.NewScheduler(repo, application.NewSearchService(repo, nil, nil, &countingEnqueuer{}), nil, nil)

	s.Tick(context.Background())

	if repo.subRunStarted {
		t.Error("expected a lost subscription claim to skip the run entirely")
	}
}

// manualSubRepo answers the two reads the run paths make, so a manual row can
// be offered to them directly. The production due-query filters manual rows out
// in SQL (ListEnabledSubscriptions); these tests cover the service-level refusal
// that has to hold even when a caller reaches past the scheduler (041 FR-014).
type manualSubRepo struct {
	domain.SearchRepository
	subs []sqlcgen.Subscription
}

func (f *manualSubRepo) ListSubscriptions(ctx context.Context) ([]sqlcgen.Subscription, error) {
	return f.subs, nil
}

func (f *manualSubRepo) GetSubscription(ctx context.Context, id pgtype.UUID) (sqlcgen.Subscription, error) {
	for _, s := range f.subs {
		if s.ID == id {
			return s, nil
		}
	}
	return sqlcgen.Subscription{}, pgx.ErrNoRows
}

func manualSubscription() sqlcgen.Subscription {
	return sqlcgen.Subscription{
		ID:        pgtype.UUID{Bytes: [16]byte{11}, Valid: true},
		SourceKey: "djinni",
		Url:       "",
		Enabled:   true,
		Cron:      "0 */6 * * *",
		Kind:      "manual",
	}
}

func TestRunSubscription_RefusesManualRow(t *testing.T) {
	repo := &manualSubRepo{subs: []sqlcgen.Subscription{manualSubscription()}}
	enq := &countingEnqueuer{}
	svc := application.NewSearchService(repo, nil, nil, enq)

	err := svc.RunSubscription(context.Background(), "0b000000-0000-0000-0000-000000000000")

	if err == nil {
		t.Fatal("expected a manual subscription to be refused, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to crawl") {
		t.Fatalf("expected the refusal to name the reason, got %v", err)
	}
	if enq.count != 0 {
		t.Errorf("expected nothing enqueued for a manual subscription, got %d tasks", enq.count)
	}
}

func TestRunAllSubscriptions_SkipsManualRows(t *testing.T) {
	repo := &manualSubRepo{subs: []sqlcgen.Subscription{manualSubscription()}}
	enq := &countingEnqueuer{}
	svc := application.NewSearchService(repo, nil, nil, enq)

	queued, err := svc.RunAllSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("RunAllSubscriptions: %v", err)
	}
	if queued != 0 {
		t.Errorf("expected run-all to skip manual rows, got %d queued", queued)
	}
	if enq.count != 0 {
		t.Errorf("expected nothing enqueued, got %d tasks", enq.count)
	}
}
