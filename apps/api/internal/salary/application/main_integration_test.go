//go:build integration

package application_test

import (
	"context"
	"os"
	"testing"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/dbtest"
)

// TestMain holds the shared-database advisory lock for this suite. The salary
// integration tests insert a "Job" row and read it back; other packages
// TRUNCATE "Job" during their own setup, and `go test ./...` runs packages in
// parallel, so without the lock those truncations delete this suite's fixture
// mid-test (see internal/dbtest/lock.go).
func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}
	if err := db.Migrate(dsn); err != nil {
		panic("migrate: " + err.Error())
	}
	database, err := db.Open(context.Background(), dsn)
	if err != nil {
		panic("db open: " + err.Error())
	}
	defer database.Close()

	unlock, err := dbtest.LockSharedDB(context.Background(), database.Pool)
	if err != nil {
		panic("lock shared db: " + err.Error())
	}
	defer unlock()

	os.Exit(m.Run())
}
