package events

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

// LedgerStore is the subset of *sqlcgen.Queries idempotency admission needs.
// Callers pass a queries handle scoped to whatever transaction they are
// running the result write in — Admit's insert MUST land in the same
// transaction as the result it admits (data-model.md § 7).
type LedgerStore interface {
	InsertIdempotencyLedgerEntry(ctx context.Context, arg sqlcgen.InsertIdempotencyLedgerEntryParams) (sqlcgen.IdempotencyLedger, error)
	GetIdempotencyLedgerEntry(ctx context.Context, idempotencyKey string) (sqlcgen.IdempotencyLedger, error)
}

// Disposition is Admit's verdict on a result event.
type Disposition string

const (
	// Accepted is the first sighting of this idempotency key. The caller
	// should persist the result in the same transaction as the ledger write.
	Accepted Disposition = "accepted"
	// Duplicate is a redelivery of a result already accepted under the same
	// run_id (FR-030). The caller MUST discard it without persisting.
	Duplicate Disposition = "duplicate"
	// Superseded means the ledger already holds a different run_id for this
	// key — an earlier attempt's result arriving after a later attempt's
	// result already won (FR-037). The caller MUST discard it.
	Superseded Disposition = "superseded"
)

// supersededTotal counts results discarded as Superseded, for
// events/metrics.go (M8-1) to expose.
var supersededTotal atomic.Int64

// SupersededTotal reports how many result events have been discarded as
// superseded since process start (FR-037).
func SupersededTotal() int64 {
	return supersededTotal.Load()
}

// Admit writes (idempotencyKey, runID) to the idempotency ledger through
// store and reports whether the caller should proceed with persisting the
// result (data-model.md § 7). A duplicate key discards (FR-030); a run_id
// mismatch against the stored row discards and counts as superseded
// (FR-037). store MUST be scoped to the same transaction the caller commits
// the result in, so admission and persistence are atomic.
func Admit(ctx context.Context, store LedgerStore, workType, idempotencyKey, runID string) (Disposition, error) {
	runUUID, err := dbutil.ParseUUID(runID)
	if err != nil {
		return "", err
	}

	_, err = store.InsertIdempotencyLedgerEntry(ctx, sqlcgen.InsertIdempotencyLedgerEntryParams{
		IdempotencyKey: idempotencyKey,
		WorkType:       workType,
		RunID:          runUUID,
	})
	if err == nil {
		return Accepted, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	// ON CONFLICT DO NOTHING returned no row: the key was already present.
	existing, err := store.GetIdempotencyLedgerEntry(ctx, idempotencyKey)
	if err != nil {
		return "", err
	}
	if existing.RunID == runUUID {
		return Duplicate, nil
	}
	supersededTotal.Add(1)
	return Superseded, nil
}
