//go:build integration

package http_test

import (
	"context"
	"net/http"
	"testing"

	activityhttp "github.com/job-finder/api/internal/activity/interfaces/http"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/testutil"
)

func TestActivityList(t *testing.T) {
	ctx := context.Background()

	// Own database per suite: no shared tables, so no truncation and no
	// cross-package coordination (internal/dbtest).
	database := dbtest.New(t)

	// Insert a mock activity
	_, err := database.Queries.InsertActivityRun(ctx, sqlcgen.InsertActivityRunParams{
		Op:    "ingest",
		Label: "test scrape",
		Meta:  []byte("{}"),
	})
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	h := activityhttp.NewActivityHandler(database.Queries, nil, nil, nil, nil)
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
