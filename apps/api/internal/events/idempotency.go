package events

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type LedgerStore interface {
	InsertIdempotencyLedgerEntry(ctx context.Context, arg sqlcgen.InsertIdempotencyLedgerEntryParams) (sqlcgen.IdempotencyLedger, error)
	GetIdempotencyLedgerEntry(ctx context.Context, idempotencyKey string) (sqlcgen.IdempotencyLedger, error)
}

type Disposition string

const (
	Accepted Disposition = "accepted"

	Duplicate Disposition = "duplicate"

	Superseded Disposition = "superseded"
)

var supersededTotal atomic.Int64

func SupersededTotal() int64 {
	return supersededTotal.Load()
}

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
