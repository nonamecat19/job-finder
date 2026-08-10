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

// TestGetGenerationWorkspace_ItemsInPositionOrderAndByteIdenticalText is
// SC-001's measurement (rest-api.md's three response rules): items are
// returned in `position` order, and every `origin: "profile"` item's `text`
// is byte-identical to the master bullet at its `sourceIndex`.
func TestGetGenerationWorkspace_ItemsInPositionOrderAndByteIdenticalText(t *testing.T) {
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
	if getW.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var run dto.GenerationRunDto
	testutil.ParseJSON(getW, &run)

	var expSection *dto.GenerationSectionDto
	for i := range run.Sections {
		if run.Sections[i].Kind == "experience" {
			expSection = &run.Sections[i]
		}
	}
	if expSection == nil {
		t.Fatal("expected an experience section")
	}
	if len(expSection.Items) != len(bullets) {
		t.Fatalf("expected %d items (every master bullet), got %d", len(bullets), len(expSection.Items))
	}

	// Items are returned in `position` order.
	for i := 1; i < len(expSection.Items); i++ {
		if expSection.Items[i-1].Position > expSection.Items[i].Position {
			t.Fatalf("items not in position order: %+v", expSection.Items)
		}
	}

	// Every profile-origin item's text is byte-identical to the master
	// bullet at its sourceIndex.
	for _, it := range expSection.Items {
		if it.Origin != "profile" {
			t.Errorf("item %+v origin = %q, want profile (Phase 3 has no AI ranking yet)", it, it.Origin)
			continue
		}
		if it.SourceIndex == nil {
			t.Errorf("item %+v has no sourceIndex", it)
			continue
		}
		if *it.SourceIndex < 0 || *it.SourceIndex >= len(bullets) {
			t.Fatalf("item %+v sourceIndex %d out of range [0,%d)", it, *it.SourceIndex, len(bullets))
		}
		want := bullets[*it.SourceIndex]
		if it.Text != want {
			t.Errorf("item text = %q, want byte-identical to master bullet %q", it.Text, want)
		}
	}
}
