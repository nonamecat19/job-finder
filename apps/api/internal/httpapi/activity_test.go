//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

func TestActivityList(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}

	ctx := context.Background()
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	database, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer database.Close()

	// Clear activities
	_, _ = database.Pool.Exec(ctx, `TRUNCATE TABLE "ActivityRun" CASCADE`)

	// Insert a mock activity
	_, err = database.Queries.InsertActivityRun(ctx, sqlcgen.InsertActivityRunParams{
		Op:    "ingest",
		Label: "test scrape",
		Meta:  []byte("{}"),
	})
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	h := httpapi.NewActivityHandler(database.Queries, nil, nil)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/activity", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Active []dto.ActivityRunDto `json:"active"`
		Recent []dto.ActivityRunDto `json:"recent"`
	}
	testutil.ParseJSON(w, &resp)

	if len(resp.Active) != 1 {
		t.Errorf("expected 1 active run, got %d", len(resp.Active))
	}
	if len(resp.Recent) != 1 {
		t.Errorf("expected 1 recent run, got %d", len(resp.Recent))
	}
	if resp.Active[0].Label != "test scrape" {
		t.Errorf("expected label 'test scrape', got %q", resp.Active[0].Label)
	}
}
