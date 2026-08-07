package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func TestSanitize(t *testing.T) {
	got := sanitize("Acme Corp / Special Chars!! — Senior Engineer")
	want := "acme_corp_special_chars_senior_engineer"
	if got != want {
		t.Errorf("sanitize() = %q, want %q", got, want)
	}
}

type fakeRenderer struct {
	pages     []int
	renders   []string
	expands   int
	condenses int
	expandErr error
}

func (f *fakeRenderer) deps() renderDeps {
	return renderDeps{
		render: func(_ context.Context, _ domain.RendercvMaster, name string) (string, error) {
			f.renders = append(f.renders, name)
			return "/tmp/" + name + ".pdf", nil
		},
		countPages: func(string) (int, error) {
			i := len(f.renders) - 1
			if i < len(f.pages) {
				return f.pages[i], nil
			}
			return f.pages[len(f.pages)-1], nil
		},
		expand: func(context.Context, domain.RendercvMaster, domain.VacancyAnalysis, domain.GroundingLevel, domain.ShapeConfig) (domain.TailoredSections, error) {
			f.expands++
			if f.expandErr != nil {
				return domain.TailoredSections{}, f.expandErr
			}
			return domain.TailoredSections{Summary: "expanded"}, nil
		},
		condense: func(context.Context, domain.RendercvMaster, domain.VacancyAnalysis, domain.GroundingLevel, domain.ShapeConfig) (domain.TailoredSections, error) {
			f.condenses++
			return domain.TailoredSections{Summary: "condensed"}, nil
		},
	}
}

func testMaster() domain.RendercvMaster {
	return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"summary": []any{"A summary."},
		"experience": []any{map[string]any{
			"company":    "Acme",
			"highlights": []any{"Did a thing"},
		}},
	}}}
}

func renderWith(t *testing.T, f *fakeRenderer, cfg domain.ShapeConfig) renderOutcome {
	t.Helper()
	s := &Service{}
	out, err := s.renderToPageTarget(context.Background(), f.deps(), testMaster(), testMaster(), domain.VacancyAnalysis{}, domain.GroundingModerate, cfg, "base", nil)
	if err != nil {
		t.Fatalf("renderToPageTarget: %v", err)
	}
	return out
}

func TestRenderStopsWhenTargetIsMetOnFirstRender(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	cfg.TargetPages = 1
	f := &fakeRenderer{pages: []int{1}}

	out := renderWith(t, f, cfg)

	if f.expands != 0 || f.condenses != 0 {
		t.Errorf("expands = %d, condenses = %d, want 0 and 0 — the target was already met", f.expands, f.condenses)
	}
	if len(f.renders) != 1 {
		t.Errorf("renders = %v, want a single render", f.renders)
	}
	if out.pages != 1 {
		t.Errorf("achieved pages = %d, want 1", out.pages)
	}
}

func TestRenderExpandsWhenUnderTarget(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	f := &fakeRenderer{pages: []int{1, 2}}

	out := renderWith(t, f, cfg)

	if f.expands != 1 {
		t.Errorf("expands = %d, want 1", f.expands)
	}
	if f.condenses != 0 {
		t.Errorf("condenses = %d, want 0", f.condenses)
	}
	if out.pages != 2 {
		t.Errorf("achieved pages = %d, want 2", out.pages)
	}
	if !strings.HasSuffix(out.pdfPath, "base-expanded.pdf") {
		t.Errorf("pdfPath = %q, want the expanded render", out.pdfPath)
	}
	if out.conflict {
		t.Error("conflict = true, want false — the expand path does not reduce section lengths")
	}
}

func TestRenderCompactsThenCondensesWhenOverTarget(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	f := &fakeRenderer{pages: []int{3, 3, 2}}

	out := renderWith(t, f, cfg)

	if f.condenses != 1 {
		t.Errorf("condenses = %d, want 1", f.condenses)
	}
	if f.expands != 0 {
		t.Errorf("expands = %d, want 0", f.expands)
	}
	if want := []string{"base", "base-compact", "base-condensed"}; !equalStrings(f.renders, want) {
		t.Errorf("renders = %v, want %v", f.renders, want)
	}
	if out.pages != 2 {
		t.Errorf("achieved pages = %d, want 2", out.pages)
	}
	if !out.conflict {
		t.Error("conflict = false, want true — condensing overrode the configured section lengths (FR-016)")
	}
}

func TestRenderCompactAloneCanMeetTarget(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	f := &fakeRenderer{pages: []int{3, 2}}

	out := renderWith(t, f, cfg)

	if f.condenses != 0 {
		t.Errorf("condenses = %d, want 0 — the compact design already met the target", f.condenses)
	}
	if out.pages != 2 {
		t.Errorf("achieved pages = %d, want 2", out.pages)
	}
	if out.conflict {
		t.Error("conflict = true, want false — no condense ran")
	}
}

func TestRenderReturnsBestResultWhenTargetIsUnreachable(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	cfg.TargetPages = 1
	f := &fakeRenderer{pages: []int{4, 3, 3}}

	out := renderWith(t, f, cfg)

	if len(f.renders) != 3 {
		t.Errorf("renders = %v, want 3 (initial + %d bounded attempts)", f.renders, shapeAttempts)
	}
	if out.pages != 3 {
		t.Errorf("achieved pages = %d, want the best reached (3)", out.pages)
	}
	if out.pdfPath == "" {
		t.Error("pdfPath is empty, want the best result reached — an unreachable target must not fail the run")
	}
}

func TestRenderReturnsShortVersionWhenExpandFails(t *testing.T) {
	cfg := domain.DefaultShapeConfig()
	f := &fakeRenderer{pages: []int{1}, expandErr: errors.New("llm down")}

	out := renderWith(t, f, cfg)

	if out.pages != 1 {
		t.Errorf("achieved pages = %d, want the 1-page version returned", out.pages)
	}
	if !strings.HasSuffix(out.pdfPath, "base.pdf") {
		t.Errorf("pdfPath = %q, want the original render", out.pdfPath)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
