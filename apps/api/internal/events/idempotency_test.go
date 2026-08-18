//go:build integration

package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/events"
)

// TestIdempotency_Integration_RedeliveryProducesExactlyOneAccept proves the
// core guarantee T032 asks for: a redelivered work event's result is
// admitted once. The first Admit, run in its own transaction exactly as a
// consumer would run it, is the one that gets to persist a result
// (Accepted). A redelivery — same idempotency_key, same run_id, a fresh
// transaction because it is a separate consumer invocation — is recognised
// as a Duplicate and MUST NOT persist a second result (FR-030).
func TestIdempotency_Integration_RedeliveryProducesExactlyOneAccept(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	key := "ghost:job-1:" + uuid.NewString()
	runID := uuid.NewString()

	// Three redeliveries of the identical work event, each its own
	// transaction (a separate consumer invocation per delivery).
	dispositions := make([]events.Disposition, 0, 3)
	for i := 0; i < 3; i++ {
		var got events.Disposition
		err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			d, err := events.Admit(ctx, q, "ghost", key, runID)
			if err != nil {
				return err
			}
			got = d
			return nil
		})
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		dispositions = append(dispositions, got)
	}

	accepts := 0
	for _, d := range dispositions {
		if d == events.Accepted {
			accepts++
		}
	}
	if accepts != 1 {
		t.Fatalf("expected exactly one Accepted disposition across redeliveries, got dispositions=%v", dispositions)
	}
	if dispositions[0] != events.Accepted {
		t.Errorf("expected the first delivery to be Accepted, got %s", dispositions[0])
	}
	for _, d := range dispositions[1:] {
		if d != events.Duplicate {
			t.Errorf("expected redeliveries to be Duplicate, got %s", d)
		}
	}
}

// TestIdempotency_Integration_SupersededRunDiscarded proves FR-037: a result
// from an earlier attempt (a different run_id under the same idempotency
// key) arriving after a later attempt already won is discarded and counted,
// never overwriting the accepted row.
func TestIdempotency_Integration_SupersededRunDiscarded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	key := "ghost:job-2:" + uuid.NewString()
	acceptedRun := uuid.NewString()
	staleRun := uuid.NewString()

	before := events.SupersededTotal()

	var first events.Disposition
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		d, err := events.Admit(ctx, q, "ghost", key, acceptedRun)
		first = d
		return err
	}); err != nil {
		t.Fatalf("first admit: %v", err)
	}
	if first != events.Accepted {
		t.Fatalf("expected first admit to be Accepted, got %s", first)
	}

	var second events.Disposition
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		d, err := events.Admit(ctx, q, "ghost", key, staleRun)
		second = d
		return err
	}); err != nil {
		t.Fatalf("second admit: %v", err)
	}
	if second != events.Superseded {
		t.Fatalf("expected the differing run_id to be Superseded, got %s", second)
	}

	after := events.SupersededTotal()
	if after <= before {
		t.Errorf("expected SupersededTotal to increase, before=%d after=%d", before, after)
	}

	entry, err := testDB.Queries.GetIdempotencyLedgerEntry(ctx, key)
	if err != nil {
		t.Fatalf("get ledger entry: %v", err)
	}
	if got := dbutil.UUIDString(entry.RunID); got != acceptedRun {
		t.Errorf("ledger names run %q, expected the accepted run %q — superseded write must not overwrite it", got, acceptedRun)
	}
}

// TestIdempotency_Integration_RollbackAllowsReadmission proves the
// transactional requirement in data-model.md § 7: the ledger write lands in
// the same transaction as the result it admits. If that transaction rolls
// back (the "persist the result" half of the work fails), the ledger row
// never commits either, and the next delivery is Accepted again rather than
// wrongly deduped.
func TestIdempotency_Integration_RollbackAllowsReadmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	key := "ghost:job-3:" + uuid.NewString()
	runID := uuid.NewString()

	failPersist := errors.New("simulated result-persist failure")
	err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		d, aerr := events.Admit(ctx, q, "ghost", key, runID)
		if aerr != nil {
			return aerr
		}
		if d != events.Accepted {
			t.Fatalf("expected Accepted, got %s", d)
		}
		return failPersist
	})
	if !errors.Is(err, failPersist) {
		t.Fatalf("expected WithinTx to surface the persist failure, got %v", err)
	}

	var retryDisposition events.Disposition
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		d, aerr := events.Admit(ctx, q, "ghost", key, runID)
		retryDisposition = d
		return aerr
	}); err != nil {
		t.Fatalf("retry admit: %v", err)
	}
	if retryDisposition != events.Accepted {
		t.Errorf("expected retry after rollback to be Accepted, got %s", retryDisposition)
	}
}
