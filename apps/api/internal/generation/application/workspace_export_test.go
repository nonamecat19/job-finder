package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func exportMaster() domain.RendercvMaster {
	return domain.RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"summary": []any{"Master summary."},
				"experience": []any{
					map[string]any{
						"company": "Acme", "position": "Senior Engineer",
						"highlights": []any{"Shipped the thing", "Fixed the other thing", "Wrote the docs"},
					},
				},
				"skills": []any{map[string]any{"label": "Languages", "details": "Go"}},
			},
		},
	}
}

func exportSections() []domain.Section {
	entry := "Acme"
	return []domain.Section{
		{
			ID: "sec-exp", Kind: domain.SectionKindExperience, EntryKey: &entry, Position: 0, Enabled: true,
			Items: []domain.Item{
				{ID: "i-0", Origin: domain.OriginProfile, SourceIndex: intPtr(0), SourceText: "Shipped the thing", Rank: 0, Position: 0, Selected: true},
				{ID: "i-1", Origin: domain.OriginProfile, SourceIndex: intPtr(1), SourceText: "Fixed the other thing", Rank: 1, Position: 1, Selected: true},
				{ID: "i-2", Origin: domain.OriginProfile, SourceIndex: intPtr(2), SourceText: "Wrote the docs", Rank: 2, Position: 2, Selected: true},
			},
		},
	}
}

func intPtr(i int) *int { return &i }

func exportDeps(t *testing.T, pages []int) (renderDeps, *int) {
	t.Helper()
	renders := 0
	return renderDeps{
		render: func(context.Context, domain.RendercvMaster, string) (string, error) {
			renders++
			return "/tmp/export.pdf", nil
		},
		countPages: func(string) (int, error) {
			i := renders - 1
			if i >= len(pages) {
				i = len(pages) - 1
			}
			return pages[i], nil
		},
		expand: func(context.Context, domain.RendercvMaster, domain.VacancyAnalysis, domain.GroundingLevel, domain.ShapeConfig) (domain.TailoredSelection, error) {
			t.Error("the export path called expand: an LLM call after the user approved the selection (FR-018)")
			return domain.TailoredSelection{}, nil
		},
		condense: func(doc domain.RendercvMaster, cfg domain.ShapeConfig) (domain.RendercvMaster, bool, error) {
			t.Error("the export path called condense: TrimHighlights silently drops approved content (FR-018)")
			return doc, false, nil
		},
	}, &renders
}

func TestWorkspaceExport(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	cfg.TargetPages = 2
	deps, renders := exportDeps(t, []int{2})

	out, err := renderExport(context.Background(), deps, exportMaster(), exportSections(), cfg, "acme")
	if err != nil {
		t.Fatalf("renderExport: %v", err)
	}
	if out.blocked {
		t.Error("a document inside the page budget was reported as blocked")
	}
	if *renders != 1 {
		t.Errorf("renders = %d, want exactly one for a document that already fits", *renders)
	}
	if got := domain.StringSliceField(exportEntry(t, out.doc), "highlights"); len(got) != 3 {
		t.Errorf("highlights = %v, want all three selected bullets", got)
	}
}

func TestWorkspaceExportCompactsLayoutOnceWhenOverTarget(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	cfg.TargetPages = 2
	deps, renders := exportDeps(t, []int{3, 2})

	out, err := renderExport(context.Background(), deps, exportMaster(), exportSections(), cfg, "acme")
	if err != nil {
		t.Fatalf("renderExport: %v", err)
	}
	if out.blocked {
		t.Error("the compacted render fit the budget but was reported as blocked")
	}
	if *renders != 2 {
		t.Errorf("renders = %d, want the original plus one compacted re-render", *renders)
	}
	if got := domain.StringSliceField(exportEntry(t, out.doc), "highlights"); len(got) != 3 {
		t.Errorf("highlights = %v, want the approved content untouched by the compact pass", got)
	}
}

func TestWorkspaceExportBlocksRatherThanTrimming(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	cfg.TargetPages = 2
	deps, renders := exportDeps(t, []int{4, 3})

	out, err := renderExport(context.Background(), deps, exportMaster(), exportSections(), cfg, "acme")
	if err != nil {
		t.Fatalf("renderExport: %v", err)
	}
	if !out.blocked {
		t.Fatal("a document over the budget after compacting was not blocked")
	}
	if *renders != 2 {
		t.Errorf("renders = %d, want exactly two — the export path renders once and re-renders once", *renders)
	}
	if len(out.candidates) == 0 {
		t.Fatal("a blocked export named no drop candidates (FR-019)")
	}
	if out.candidates[0].Rank != 2 {
		t.Errorf("first candidate = %+v, want the worst-ranked selected item", out.candidates[0])
	}
	if got := domain.StringSliceField(exportEntry(t, out.doc), "highlights"); len(got) != 3 {
		t.Errorf("highlights = %v, want nothing dropped by a blocked export", got)
	}
}

func exportEntry(t *testing.T, doc domain.RendercvMaster) map[string]any {
	t.Helper()
	entries := domain.AsSliceOfMaps(domain.CvSections(doc)["experience"])
	if len(entries) == 0 {
		t.Fatalf("no experience entries in %v", doc)
	}
	return entries[0]
}
