package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/jobs"
)

// fakeRepo embeds the Repository port so unimplemented methods satisfy the
// interface; the test overrides only what it exercises. This is the payoff of
// the hexagonal seam: the use-case is now testable without a real database.
type fakeRepo struct {
	jobs.Repository
	deleted    int64
	deleteErr  error
	deleteCall int

	// Captured for the salary-floor assertions.
	countParams sqlcgen.CountJobsParams
	listParams  sqlcgen.ListJobsByScoreParams
	rows        []sqlcgen.ListJobsByScoreRow
}

func (f *fakeRepo) DeleteAllJobs(ctx context.Context) (int64, error) {
	f.deleteCall++
	return f.deleted, f.deleteErr
}

func (f *fakeRepo) CountJobs(ctx context.Context, arg sqlcgen.CountJobsParams) (int64, error) {
	f.countParams = arg
	return int64(len(f.rows)), nil
}

func (f *fakeRepo) ListJobsByScore(ctx context.Context, arg sqlcgen.ListJobsByScoreParams) ([]sqlcgen.ListJobsByScoreRow, error) {
	f.listParams = arg
	return f.rows, nil
}

func (f *fakeRepo) ListJobSignalsByJobIds(ctx context.Context, arg sqlcgen.ListJobSignalsByJobIdsParams) ([]sqlcgen.JobSignal, error) {
	return nil, nil
}

type fakeEnqueuer struct{ jobs.Enqueuer }

func TestServiceDeleteAll(t *testing.T) {
	repo := &fakeRepo{deleted: 7}
	svc := jobs.NewService(repo, &fakeEnqueuer{}, 0)

	n, err := svc.DeleteAll(context.Background())
	if err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	if n != 7 {
		t.Errorf("deleted = %d, want 7", n)
	}
	if repo.deleteCall != 1 {
		t.Errorf("DeleteAllJobs called %d times, want 1", repo.deleteCall)
	}
}

func TestServiceDeleteAllError(t *testing.T) {
	repo := &fakeRepo{deleteErr: errors.New("boom")}
	svc := jobs.NewService(repo, &fakeEnqueuer{}, 0)

	if _, err := svc.DeleteAll(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func mkUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[0] = b
	u.Valid = true
	return u
}

func int32p(v int32) *int32 { return &v }
func strp(v string) *string { return &v }

// TestServiceList_FloorZeroOmitsPredicate covers FR-018: a zero floor must
// pass nil, not a literal 0, so the SQL predicate is inert rather than
// evaluating ">= 0".
func TestServiceList_FloorZeroOmitsPredicate(t *testing.T) {
	repo := &fakeRepo{}
	svc := jobs.NewService(repo, &fakeEnqueuer{}, 0)

	if _, err := svc.List(context.Background(), jobs.ListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.countParams.SalaryFloor != nil {
		t.Errorf("SalaryFloor = %v, want nil when floor is 0", repo.countParams.SalaryFloor)
	}
	if repo.listParams.SalaryFloor != nil {
		t.Errorf("SalaryFloor = %v, want nil when floor is 0", repo.listParams.SalaryFloor)
	}
}

// TestServiceList_FloorAppliedByDefault covers FR-015: a configured floor is
// sent to the query by default (ShowBelowFloor false).
func TestServiceList_FloorAppliedByDefault(t *testing.T) {
	repo := &fakeRepo{}
	svc := jobs.NewService(repo, &fakeEnqueuer{}, 50000)

	if _, err := svc.List(context.Background(), jobs.ListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.countParams.SalaryFloor == nil || *repo.countParams.SalaryFloor != 50000 {
		t.Errorf("CountJobs SalaryFloor = %v, want 50000", repo.countParams.SalaryFloor)
	}
	if repo.listParams.SalaryFloor == nil || *repo.listParams.SalaryFloor != 50000 {
		t.Errorf("ListJobsByScore SalaryFloor = %v, want 50000", repo.listParams.SalaryFloor)
	}
}

// TestServiceList_ShowBelowFloorOmitsPredicate covers FR-016: toggling the
// reveal flag omits the predicate even though a floor is configured.
func TestServiceList_ShowBelowFloorOmitsPredicate(t *testing.T) {
	repo := &fakeRepo{}
	svc := jobs.NewService(repo, &fakeEnqueuer{}, 50000)

	if _, err := svc.List(context.Background(), jobs.ListParams{ShowBelowFloor: true}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if repo.countParams.SalaryFloor != nil {
		t.Errorf("SalaryFloor = %v, want nil when revealing below-floor jobs", repo.countParams.SalaryFloor)
	}
}

// TestServiceList_MarksBelowFloor covers the red-marker requirement (FR-016):
// a USD band under the floor is marked, a non-USD band is never marked
// (FR-020 fails open), and a band-less job is never marked (FR-019).
func TestServiceList_MarksBelowFloor(t *testing.T) {
	repo := &fakeRepo{rows: []sqlcgen.ListJobsByScoreRow{
		{ID: mkUUID(1), SalaryMax: int32p(30000), SalaryCurrency: strp("USD")},
		{ID: mkUUID(2), SalaryMax: int32p(90000), SalaryCurrency: strp("USD")},
		{ID: mkUUID(3), SalaryMax: int32p(30000), SalaryCurrency: strp("UAH")},
		{ID: mkUUID(4)},
	}}
	svc := jobs.NewService(repo, &fakeEnqueuer{}, 50000)

	out, err := svc.List(context.Background(), jobs.ListParams{ShowBelowFloor: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out.Items) != 4 {
		t.Fatalf("items = %d, want 4", len(out.Items))
	}
	want := map[int]bool{0: true, 1: false, 2: false, 3: false}
	for i, w := range want {
		if out.Items[i].SalaryBelowFloor != w {
			t.Errorf("item %d SalaryBelowFloor = %v, want %v", i, out.Items[i].SalaryBelowFloor, w)
		}
	}
}
