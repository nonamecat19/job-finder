//go:build integration

package http_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/testutil"
)

func firstItemID(t *testing.T, r chi.Router, runID, kind string) string {
	t.Helper()
	w := testutil.DoRequest(r, "GET", "/api/generations/"+runID, nil, map[string]string{"runId": runID})
	if w.Code != 200 {
		t.Fatalf("GET run: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var run dto.GenerationRunDto
	testutil.ParseJSON(w, &run)
	for _, sec := range run.Sections {
		if sec.Kind == kind && len(sec.Items) > 0 {
			return sec.Items[0].ID
		}
	}
	t.Fatalf("no item found in a %q section for run %q: %+v", kind, runID, run)
	return ""
}

func TestPatchGenerationItem_ForbidsTextOnProfileItem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.", []string{"Shipped the thing"}, "Languages", "Go")
	prof := insertTestProfile(ctx, t, testDB, config)
	h, svc := newWorkspaceHandler(t, testDB)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/generations", map[string]any{
		"profileId": dbutil.UUIDString(prof.ID),
		"vacancy":   map[string]any{"company": "Acme", "title": "Senior Engineer", "text": "Looking for a Go engineer."},
	}, nil)
	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var started struct {
		RunID string `json:"runId"`
	}
	testutil.ParseJSON(w, &started)

	if err := svc.StartRun(ctx, started.RunID, nil); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	itemID := firstItemID(t, r, started.RunID, "experience")

	patchW := testutil.DoRequestJSON(r, "PATCH", "/api/generations/"+started.RunID+"/items/"+itemID,
		map[string]any{"text": "a different bullet"},
		map[string]string{"runId": started.RunID, "itemId": itemID})
	if patchW.Code != 403 {
		t.Fatalf("expected 403 for text on a profile-origin item, got %d: %s", patchW.Code, patchW.Body.String())
	}
}

func TestPatchGenerationItem_ConflictWhenRunning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.", []string{"Shipped the thing"}, "Languages", "Go")
	prof := insertTestProfile(ctx, t, testDB, config)
	h, _ := newWorkspaceHandler(t, testDB)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/generations", map[string]any{
		"profileId": dbutil.UUIDString(prof.ID),
		"vacancy":   map[string]any{"company": "Acme", "title": "Senior Engineer", "text": "Looking for a Go engineer."},
	}, nil)
	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var started struct {
		RunID string `json:"runId"`
	}
	testutil.ParseJSON(w, &started)

	itemID := firstItemID(t, r, started.RunID, "experience")

	patchW := testutil.DoRequestJSON(r, "PATCH", "/api/generations/"+started.RunID+"/items/"+itemID,
		map[string]any{"selected": false},
		map[string]string{"runId": started.RunID, "itemId": itemID})
	if patchW.Code != 409 {
		t.Fatalf("expected 409 while the run is running, got %d: %s", patchW.Code, patchW.Body.String())
	}
}

func TestPatchGenerationItem_IdempotentOnRepeatedBody(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.", []string{"Shipped the thing", "Fixed the other thing"}, "Languages", "Go")
	prof := insertTestProfile(ctx, t, testDB, config)
	h, svc := newWorkspaceHandler(t, testDB)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/generations", map[string]any{
		"profileId": dbutil.UUIDString(prof.ID),
		"vacancy":   map[string]any{"company": "Acme", "title": "Senior Engineer", "text": "Looking for a Go engineer."},
	}, nil)
	var started struct {
		RunID string `json:"runId"`
	}
	testutil.ParseJSON(w, &started)
	if err := svc.StartRun(ctx, started.RunID, nil); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	itemID := firstItemID(t, r, started.RunID, "experience")

	body := map[string]any{"selected": false, "position": 1}
	first := testutil.DoRequestJSON(r, "PATCH", "/api/generations/"+started.RunID+"/items/"+itemID, body,
		map[string]string{"runId": started.RunID, "itemId": itemID})
	if first.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", first.Code, first.Body.String())
	}
	var firstItem dto.GenerationItemDto
	testutil.ParseJSON(first, &firstItem)

	second := testutil.DoRequestJSON(r, "PATCH", "/api/generations/"+started.RunID+"/items/"+itemID, body,
		map[string]string{"runId": started.RunID, "itemId": itemID})
	if second.Code != 200 {
		t.Fatalf("expected 200 on the repeated identical body, got %d: %s", second.Code, second.Body.String())
	}
	var secondItem dto.GenerationItemDto
	testutil.ParseJSON(second, &secondItem)

	if firstItem.ID != secondItem.ID || firstItem.Selected != secondItem.Selected ||
		firstItem.Position != secondItem.Position || firstItem.Text != secondItem.Text {
		t.Fatalf("expected the repeated PATCH to be a no-op returning the same row, got %+v then %+v", firstItem, secondItem)
	}
	if secondItem.Selected {
		t.Fatalf("expected selected = false after the PATCH, got true")
	}
}
