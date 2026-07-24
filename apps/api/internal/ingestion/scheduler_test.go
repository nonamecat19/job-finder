package ingestion_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/ingestion"
)

// schedulerFakeRepo serves one due search and records whether the scheduler
// went on to read it back — GetSavedSearch is RunSearch's first call, so it
// stands in for "the run actually started" without needing a live registry.
type schedulerFakeRepo struct {
	ingestion.Repository
	search      sqlcgen.SavedSearch
	claimErr    error
	claimedWith pgtype.Timestamp
	claimCalls  int
	runStarted  bool
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
	return f.search, pgx.ErrNoRows // stop RunSearch before it needs a registry
}

func (f *schedulerFakeRepo) DeleteActivityRunsBefore(ctx context.Context, createdat pgtype.Timestamp) error {
	return nil
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

// The scheduler claims a due search before running it, passing the lastRunAt
// it made the due decision on so the write is a compare-and-swap.
func TestTick_ClaimsBeforeRunning(t *testing.T) {
	repo := &schedulerFakeRepo{search: dueSearch()}
	s := ingestion.NewScheduler(repo, ingestion.NewService(repo, nil, nil, &fakeEnqueuer{}))

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

// A lost claim means another scheduler (or another tick) already took this
// slot — the search must not be scraped a second time.
func TestTick_LostClaimSkipsRun(t *testing.T) {
	repo := &schedulerFakeRepo{search: dueSearch(), claimErr: pgx.ErrNoRows}
	s := ingestion.NewScheduler(repo, ingestion.NewService(repo, nil, nil, &fakeEnqueuer{}))

	s.Tick(context.Background())

	if repo.runStarted {
		t.Error("expected a lost claim to skip the run entirely")
	}
}
