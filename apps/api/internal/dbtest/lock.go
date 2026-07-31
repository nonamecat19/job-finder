//go:build integration

// Package dbtest holds shared helpers for the integration test suites. It is
// compiled only under the `integration` build tag and is never linked into the
// production binary.
package dbtest

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sharedDBLockKey namespaces the advisory lock every integration package takes.
const sharedDBLockKey = 0x104B_F1DE

// lockWaitTimeout bounds how long a suite waits for its turn. Every integration
// package queues on the same lock, so the wait grows with the number of suites
// and with the slowest suite's runtime — it is unrelated to the caller's own
// per-test budget and must be far larger than it.
const lockWaitTimeout = 5 * time.Minute

// releaseTimeout bounds the unlock round-trip.
const releaseTimeout = 10 * time.Second

// LockSharedDB serialises integration test packages that share one database.
//
// `go test ./...` runs packages in parallel, and several suites TRUNCATE the
// same tables ("Job", "JobSource", "Application", …) during setup. Without
// coordination one package's cleanup wipes another package's fixtures
// mid-test, producing failures that look like flakes. Each suite takes this
// session-level advisory lock in TestMain (or at the top of the test) and holds
// it for the duration, so the suites take turns.
//
// The caller's ctx is used only for cancellation of the wait, never for its
// deadline: waiting for the lock can outlast any single test's budget, so a
// suite must start its own work budget *after* this call returns.
//
// The returned release function unlocks and returns the connection to the pool.
// It uses its own context so it still runs after the caller's ctx expired.
// Callers that end with os.Exit (TestMain) can ignore it — Postgres drops a
// session advisory lock when the connection closes on process exit.
func LockSharedDB(ctx context.Context, pool *pgxpool.Pool) (func(), error) {
	waitCtx, cancelWait := context.WithTimeout(context.WithoutCancel(ctx), lockWaitTimeout)
	defer cancelWait()

	conn, err := pool.Acquire(waitCtx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(waitCtx, "SELECT pg_advisory_lock($1)", sharedDBLockKey); err != nil {
		conn.Release()
		return nil, err
	}
	return func() {
		releaseCtx, cancelRelease := context.WithTimeout(context.Background(), releaseTimeout)
		defer cancelRelease()
		_, _ = conn.Exec(releaseCtx, "SELECT pg_advisory_unlock($1)", sharedDBLockKey)
		conn.Release()
	}, nil
}
