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
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/testutil"
)

func runItems(t *testing.T, r chi.Router, runID string) []dto.GenerationItemDto {
	t.Helper()
	w := testutil.DoRequest(r, "GET", "/api/generations/"+runID, nil, map[string]string{"runId": runID})
	if w.Code != 200 {
		t.Fatalf("GET run: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var run dto.GenerationRunDto
	testutil.ParseJSON(w, &run)
	var out []dto.GenerationItemDto
	for _, sec := range run.Sections {
		out = append(out, sec.Items...)
	}
	return out
}

func TestGenerationExportBlockedMutatesNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.",
		[]string{"Shipped the thing", "Fixed the other thing", "Wrote the docs"},
		"Languages", "Go, TypeScript")
	prof := insertTestProfile(ctx, t, testDB, config)
	h, svc := newWorkspaceHandler(t, testDB)

	svc.SetExportRenderer(
		func(context.Context, domain.RendercvMaster, string) (string, error) { return "/tmp/export.pdf", nil },
		func(string) (int, error) { return 5, nil },
	)

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

	before := runItems(t, r, started.RunID)

	exportW := testutil.DoRequest(r, "POST", "/api/generations/"+started.RunID+"/export", nil,
		map[string]string{"runId": started.RunID})
	if exportW.Code != 200 {
		t.Fatalf("expected 200 for a blocked export, got %d: %s", exportW.Code, exportW.Body.String())
	}
	var export dto.GenerationExportDto
	testutil.ParseJSON(exportW, &export)

	if export.Status != "blocked" {
		t.Fatalf("export status = %q, want blocked", export.Status)
	}
	if export.DocumentID != nil {
		t.Errorf("a blocked export wrote document %v", *export.DocumentID)
	}
	if export.Report == nil || len(export.Report.Candidates) == 0 {
		t.Fatalf("a blocked export named no drop candidates: %+v", export.Report)
	}
	if export.Report.PagesRendered <= export.Report.PagesTarget {
		t.Errorf("report = %+v, want it to say how far over the budget the render came out", export.Report)
	}
	for i := 1; i < len(export.Report.Candidates); i++ {
		if export.Report.Candidates[i-1].Rank < export.Report.Candidates[i].Rank {
			t.Errorf("candidates are not worst-rank-first at %d: %+v then %+v",
				i, export.Report.Candidates[i-1], export.Report.Candidates[i])
		}
	}

	after := runItems(t, r, started.RunID)
	if len(before) != len(after) {
		t.Fatalf("item count changed across a blocked export: %d then %d", len(before), len(after))
	}
	for i := range before {
		if before[i].ID != after[i].ID || before[i].Selected != after[i].Selected ||
			before[i].Position != after[i].Position || before[i].Text != after[i].Text {
			t.Errorf("a blocked export mutated item %d: %+v then %+v", i, before[i], after[i])
		}
	}

	pollW := testutil.DoRequest(r, "GET", "/api/generations/"+started.RunID+"/export", nil,
		map[string]string{"runId": started.RunID})
	if pollW.Code != 200 {
		t.Fatalf("GET export: expected 200, got %d: %s", pollW.Code, pollW.Body.String())
	}
	var polled dto.GenerationExportDto
	testutil.ParseJSON(pollW, &polled)
	if polled.Status != "blocked" || polled.Report == nil ||
		len(polled.Report.Candidates) != len(export.Report.Candidates) {
		t.Errorf("poll = %+v, want the same blocked report the POST returned", polled)
	}
}

func TestGenerationExportWritesADocument(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.", []string{"Shipped the thing"}, "Languages", "Go")
	prof := insertTestProfile(ctx, t, testDB, config)
	h, svc := newWorkspaceHandler(t, testDB)
	svc.SetExportRenderer(
		func(context.Context, domain.RendercvMaster, string) (string, error) { return "/tmp/export.pdf", nil },
		func(string) (int, error) { return 1, nil },
	)

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

	exportW := testutil.DoRequest(r, "POST", "/api/generations/"+started.RunID+"/export", nil,
		map[string]string{"runId": started.RunID})
	if exportW.Code != 202 {
		t.Fatalf("expected 202 for an export inside the budget, got %d: %s", exportW.Code, exportW.Body.String())
	}
	var export dto.GenerationExportDto
	testutil.ParseJSON(exportW, &export)
	if export.Status != "exported" || export.DocumentID == nil {
		t.Fatalf("export = %+v, want an exported document id", export)
	}

	doc, err := svc.GetDocumentDto(ctx, *export.DocumentID)
	if err != nil {
		t.Fatalf("the exported document is not readable through the documents surface: %v", err)
	}
	if doc.Type != string(dto.DocumentTypeResume) {
		t.Errorf("document type = %q, want a resume", doc.Type)
	}
}
