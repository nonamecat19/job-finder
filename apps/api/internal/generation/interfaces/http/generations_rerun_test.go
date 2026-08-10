//go:build integration

package http_test

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/testutil"
)

// T076/T077: POST .../rerun returns the SAME run id, 409s while the run is
// already `running`, and preserves a user's toggle on a matched profile item
// across the section's delete-and-recreate (data-model.md §4).
func TestRerunGenerationRun_SameRunIdAndPreservesToggle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	bullets := []string{"Shipped the thing", "Fixed the other thing", "Mentored two engineers"}
	config := masterJSON(t, "Acme Inc.", bullets, "Languages", "Go, TypeScript")
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

	getW := testutil.DoRequest(r, "GET", "/api/generations/"+started.RunID, nil, map[string]string{"runId": started.RunID})
	var run dto.GenerationRunDto
	testutil.ParseJSON(getW, &run)
	var expSection *dto.GenerationSectionDto
	for i := range run.Sections {
		if run.Sections[i].Kind == "experience" {
			expSection = &run.Sections[i]
		}
	}
	if expSection == nil || len(expSection.Items) == 0 {
		t.Fatal("expected a non-empty experience section")
	}
	// Deselect the item that started selected — a decision that must survive
	// the rerun because its bullet (same sourceIndex) still exists after.
	target := expSection.Items[0]
	patchW := testutil.DoRequestJSON(r, "PATCH", "/api/generations/"+started.RunID+"/items/"+target.ID,
		map[string]any{"selected": !target.Selected}, map[string]string{"runId": started.RunID, "itemId": target.ID})
	if patchW.Code != 200 {
		t.Fatalf("PATCH item: expected 200, got %d: %s", patchW.Code, patchW.Body.String())
	}

	// Whole-run rerun: same run id back, 202.
	rerunW := testutil.DoRequestJSON(r, "POST", "/api/generations/"+started.RunID+"/rerun",
		map[string]any{}, map[string]string{"runId": started.RunID})
	if rerunW.Code != 202 {
		t.Fatalf("rerun: expected 202, got %d: %s", rerunW.Code, rerunW.Body.String())
	}
	var rerunResp struct {
		RunID string `json:"runId"`
	}
	testutil.ParseJSON(rerunW, &rerunResp)
	if rerunResp.RunID != started.RunID {
		t.Fatalf("rerun runId = %q, want the same run id %q", rerunResp.RunID, started.RunID)
	}

	// 409 while the run is (still, or again) running.
	conflictW := testutil.DoRequestJSON(r, "POST", "/api/generations/"+started.RunID+"/rerun",
		map[string]any{}, map[string]string{"runId": started.RunID})
	if conflictW.Code != 409 {
		t.Fatalf("rerun while running: expected 409, got %d: %s", conflictW.Code, conflictW.Body.String())
	}

	// Drive the background half synchronously, the same way every other
	// test in this package drives StartRun, then confirm the toggle survived
	// the section's delete-and-recreate. A whole-run rerun targets every
	// section, so pass every id explicitly — RerunGenerationRun computed the
	// same set server-side before enqueuing, but this test calls the
	// background half directly rather than through the queue.
	allSectionIDs := make([]string, len(run.Sections))
	for i, sec := range run.Sections {
		allSectionIDs[i] = sec.ID
	}
	if err := svc.RerunRun(ctx, started.RunID, allSectionIDs, nil); err != nil {
		t.Fatalf("RerunRun: %v", err)
	}

	afterW := testutil.DoRequest(r, "GET", "/api/generations/"+started.RunID, nil, map[string]string{"runId": started.RunID})
	var after dto.GenerationRunDto
	testutil.ParseJSON(afterW, &after)
	var afterExp *dto.GenerationSectionDto
	for i := range after.Sections {
		if after.Sections[i].Kind == "experience" {
			afterExp = &after.Sections[i]
		}
	}
	if afterExp == nil {
		t.Fatal("expected an experience section after rerun")
	}
	var matched *dto.GenerationItemDto
	for i := range afterExp.Items {
		if afterExp.Items[i].SourceIndex != nil && target.SourceIndex != nil && *afterExp.Items[i].SourceIndex == *target.SourceIndex {
			matched = &afterExp.Items[i]
		}
	}
	if matched == nil {
		t.Fatal("expected the bullet at the toggled sourceIndex to still exist after rerun")
	}
	if matched.Selected != !target.Selected {
		t.Errorf("matched item Selected = %v after rerun, want %v (the toggle preserved)", matched.Selected, !target.Selected)
	}
}
