//go:build integration

package roster_test

import (
	"context"
	"testing"

	"github.com/nonamecat19/job-scraper/ports"

	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/jobsources/roster"
)

func newPortService(t *testing.T) (ports.Roster, context.Context) {
	t.Helper()
	database := dbtest.New(t)
	return roster.NewService(database.Queries, nil), context.Background()
}

func TestPortInsertThenGetRoundTripsIDsAndFields(t *testing.T) {
	port, ctx := newPortService(t)

	inserted, err := port.InsertEmployerBoard(ctx, "greenhouse", "acme", "Acme Inc", "manual")
	if err != nil {
		t.Fatalf("InsertEmployerBoard: %v", err)
	}
	if inserted.ID == "" {
		t.Fatal("InsertEmployerBoard returned an empty ID; the pgtype.UUID -> string conversion dropped it")
	}
	if inserted.Vendor != "greenhouse" || inserted.EmployerIdentifier != "acme" {
		t.Errorf("insert returned vendor/employer %q/%q, want greenhouse/acme", inserted.Vendor, inserted.EmployerIdentifier)
	}
	if inserted.DisplayName != "Acme Inc" || inserted.AddedVia != "manual" {
		t.Errorf("insert returned displayName/addedVia %q/%q, want Acme Inc/manual", inserted.DisplayName, inserted.AddedVia)
	}

	got, err := port.GetByVendorAndEmployer(ctx, "greenhouse", "acme")
	if err != nil {
		t.Fatalf("GetByVendorAndEmployer: %v", err)
	}
	if got.ID != inserted.ID {
		t.Errorf("Get returned ID %q, want %q — the UUID does not survive the round trip", got.ID, inserted.ID)
	}
}

func TestPortGetByVendorAndEmployerMissingIsZeroNotError(t *testing.T) {
	port, ctx := newPortService(t)

	got, err := port.GetByVendorAndEmployer(ctx, "greenhouse", "does-not-exist")
	if err != nil {
		t.Fatalf("GetByVendorAndEmployer on a missing board returned an error, want the zero value: %v", err)
	}
	if got.ID != "" {
		t.Errorf("missing board returned ID %q, want empty", got.ID)
	}
}

func TestPortListForRunReturnsInsertedBoard(t *testing.T) {
	port, ctx := newPortService(t)

	inserted, err := port.InsertEmployerBoard(ctx, "lever", "globex", "Globex", "manual")
	if err != nil {
		t.Fatalf("InsertEmployerBoard: %v", err)
	}

	boards, err := port.ListForRun(ctx, "lever")
	if err != nil {
		t.Fatalf("ListForRun: %v", err)
	}

	var found *ports.EmployerBoard
	for i := range boards {
		if boards[i].ID == inserted.ID {
			found = &boards[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("ListForRun(lever) did not return the inserted board %q", inserted.ID)
	}
	if found.EmployerIdentifier != "globex" {
		t.Errorf("EmployerIdentifier = %q, want globex", found.EmployerIdentifier)
	}

	if found.LastSuccessAt != nil {
		t.Errorf("LastSuccessAt = %v on a board that never ran, want nil", found.LastSuccessAt)
	}
}

func TestPortRecordRunOutcomeUpdatesCountAndTimestamp(t *testing.T) {
	port, ctx := newPortService(t)

	inserted, err := port.InsertEmployerBoard(ctx, "ashby", "initech", "Initech", "manual")
	if err != nil {
		t.Fatalf("InsertEmployerBoard: %v", err)
	}

	if err := port.RecordRunOutcome(ctx, inserted.ID, 42); err != nil {
		t.Fatalf("RecordRunOutcome: %v", err)
	}

	boards, err := port.ListForRun(ctx, "ashby")
	if err != nil {
		t.Fatalf("ListForRun: %v", err)
	}
	var found *ports.EmployerBoard
	for i := range boards {
		if boards[i].ID == inserted.ID {
			found = &boards[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("board %q missing after RecordRunOutcome", inserted.ID)
	}
	if found.LastPostingCount != 42 {
		t.Errorf("LastPostingCount = %d, want 42 (int32 -> int conversion)", found.LastPostingCount)
	}
	if found.LastSuccessAt == nil {
		t.Error("LastSuccessAt is nil after a successful run, want a timestamp (pgtype.Timestamp -> *time.Time)")
	}
}

func TestPortRecordRunOutcomeRejectsMalformedID(t *testing.T) {
	port, ctx := newPortService(t)

	if err := port.RecordRunOutcome(ctx, "not-a-uuid", 1); err == nil {
		t.Error("RecordRunOutcome accepted a malformed ID, want a parse error")
	}
}

func TestPortDeleteEmployerBoardRemovesIt(t *testing.T) {
	port, ctx := newPortService(t)

	inserted, err := port.InsertEmployerBoard(ctx, "workable", "hooli", "Hooli", "manual")
	if err != nil {
		t.Fatalf("InsertEmployerBoard: %v", err)
	}
	if err := port.DeleteEmployerBoard(ctx, inserted.ID); err != nil {
		t.Fatalf("DeleteEmployerBoard: %v", err)
	}

	got, err := port.GetByVendorAndEmployer(ctx, "workable", "hooli")
	if err != nil {
		t.Fatalf("GetByVendorAndEmployer after delete: %v", err)
	}
	if got.ID != "" {
		t.Errorf("board still present after delete: %q", got.ID)
	}
}

func TestPortBoardCandidateRoundTrip(t *testing.T) {
	port, ctx := newPortService(t)

	inserted, err := port.InsertBoardCandidate(ctx, "greenhouse", "umbrella")
	if err != nil {
		t.Fatalf("InsertBoardCandidate: %v", err)
	}
	if inserted.ID == "" {
		t.Fatal("InsertBoardCandidate returned an empty ID")
	}

	byID, err := port.GetBoardCandidateByID(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetBoardCandidateByID: %v", err)
	}
	if byID.EmployerIdentifier != "umbrella" {
		t.Errorf("EmployerIdentifier = %q, want umbrella", byID.EmployerIdentifier)
	}

	byVendor, err := port.GetBoardCandidate(ctx, "greenhouse", "umbrella")
	if err != nil {
		t.Fatalf("GetBoardCandidate: %v", err)
	}
	if byVendor.ID != inserted.ID {
		t.Errorf("GetBoardCandidate ID = %q, want %q", byVendor.ID, inserted.ID)
	}

	if err := port.DecideBoardCandidate(ctx, inserted.ID, "accepted"); err != nil {
		t.Fatalf("DecideBoardCandidate: %v", err)
	}
	decided, err := port.GetBoardCandidateByID(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetBoardCandidateByID after decide: %v", err)
	}
	if decided.State != "accepted" {
		t.Errorf("State = %q after DecideBoardCandidate, want accepted", decided.State)
	}
}

func TestPortGetBoardCandidateMissingIsZeroNotError(t *testing.T) {
	port, ctx := newPortService(t)

	got, err := port.GetBoardCandidate(ctx, "greenhouse", "nobody")
	if err != nil {
		t.Fatalf("GetBoardCandidate on a missing candidate returned an error, want the zero value: %v", err)
	}
	if got.ID != "" {
		t.Errorf("missing candidate returned ID %q, want empty", got.ID)
	}
}
