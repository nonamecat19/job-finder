//go:build integration

// Package dbtest holds shared helpers for the integration test suites. It is
// compiled only under the `integration` build tag and is never linked into the
// production binary.
package dbtest

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sharedDBLockKey namespaces the advisory lock every integration package takes.
const sharedDBLockKey = 0x104B_F1DE

// LockSharedDB serialises integration test packages that share one database.
//
// `go test ./...` runs packages in parallel, and several suites TRUNCATE the
// same tables ("Job", "JobSource", "Application", …) during setup. Without
// coordination one package's cleanup wipes another package's fixtures
// mid-test, producing failures that look like flakes. Each suite takes this
// session-level advisory lock in TestMain (or at the top of the test) and holds
// it for the duration, so the suites take turns.
//
// The returned release function unlocks and returns the connection to the pool.
// Callers that end with os.Exit (TestMain) can ignore it — Postgres drops a
// session advisory lock when the connection closes on process exit.
func LockSharedDB(ctx context.Context, pool *pgxpool.Pool) (func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", sharedDBLockKey); err != nil {
		conn.Release()
		return nil, err
	}
	return func() {
		_, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", sharedDBLockKey)
		conn.Release()
	}, nil
}
