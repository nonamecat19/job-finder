package domain

import (
	"fmt"
	"testing"
)

// RankProjects and TrimProjectHighlights are the projects-section counterparts
// of RankSkills and TrimSkillGroups: relevance decides which projects survive
// the cap, the authored level decides how many bullets each one renders.

func projectsDoc(projects ...map[string]any) RendercvMaster {
	raw := make([]any, 0, len(projects))
	for _, p := range projects {
		raw = append(raw, p)
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"projects": raw}}}
}

func project(name string, level string, highlights ...string) map[string]any {
	hs := make([]any, 0, len(highlights))
	for _, h := range highlights {
		hs = append(hs, h)
	}
	p := map[string]any{"name": name, "highlights": hs}
	if level != "" {
		p["project_level"] = level
	}
	return p
}

func TestRankProjects_MostRelevantFirstWhenCapped(t *testing.T) {
	doc := projectsDoc(
		project("Recipe blog", "", "Built with Jekyll and Sass"),
		project("Trading engine", "", "Low-latency order matching in Go", "Kafka event pipeline"),
		project("Photo album", "", "A static gallery"),
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go", "Kafka"}}

	RankProjects(doc, analysis, ShapeConfig{ProjectsMax: 2})

	if got := projectNames(doc)[0]; got != "Trading engine" {
		t.Errorf("first project = %q, want %q", got, "Trading engine")
	}
}

// Without a cap every project renders, so the master's order is the user's own
// choice of what to lead with and nothing reorders it.
func TestRankProjects_UncappedKeepsAuthoredOrder(t *testing.T) {
	doc := projectsDoc(
		project("Recipe blog", "", "Built with Jekyll"),
		project("Trading engine", "", "Low-latency order matching in Go"),
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankProjects(doc, analysis, ShapeConfig{})

	if got := projectNames(doc)[0]; got != "Recipe blog" {
		t.Errorf("first project = %q, want the authored order to survive", got)
	}
}

// Ranking is a permutation: no project is added, dropped or reworded. Dropping
// is the cap's job, in ApplyHardLimits.
func TestRankProjects_IsAPermutation(t *testing.T) {
	doc := projectsDoc(
		project("A", "", "Go"),
		project("B", "", "Rust"),
		project("C", "", "Kafka and Go"),
	)

	RankProjects(doc, VacancyAnalysis{RequiredSkills: []string{"Go"}}, ShapeConfig{ProjectsMax: 1})

	names := projectNames(doc)
	if len(names) != 3 {
		t.Fatalf("got %d projects, want 3", len(names))
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	for _, want := range []string{"A", "B", "C"} {
		if !seen[want] {
			t.Errorf("project %q lost in the reorder", want)
		}
	}
}

func TestTrimProjectHighlights_Levels(t *testing.T) {
	bullets := make([]string, 8)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("b%d", i)
	}
	cases := []struct {
		level string
		want  int
	}{
		{ProjectLevelAll, 8},
		{ProjectLevelTop3, 3},
		{ProjectLevelTop5, 5},
		{"", 4}, // auto: half of eight
	}
	for _, c := range cases {
		t.Run("level="+c.level, func(t *testing.T) {
			doc := projectsDoc(project("P", c.level, bullets...))

			TrimProjectHighlights(doc, VacancyAnalysis{})

			got := StringSliceField(AsSliceOfMaps(CvSections(doc)["projects"])[0], "highlights")
			if len(got) != c.want {
				t.Errorf("kept %d bullets, want %d", len(got), c.want)
			}
		})
	}
}

func TestTrimProjectHighlights_RelevantKeepsWhatTheVacancyAsked(t *testing.T) {
	doc := projectsDoc(project("P", ProjectLevelRelevant,
		"Styled the marketing page",
		"Shipped a Kafka consumer",
		"Wrote the changelog",
	))

	TrimProjectHighlights(doc, VacancyAnalysis{RequiredSkills: []string{"Kafka"}})

	got := StringSliceField(AsSliceOfMaps(CvSections(doc)["projects"])[0], "highlights")
	if len(got) != 1 || got[0] != "Shipped a Kafka consumer" {
		t.Errorf("highlights = %v, want only the Kafka bullet", got)
	}
}

// A project with nothing the vacancy asked about keeps its leading bullet: a
// name with no bullets under it reads as an omission, and dropping the whole
// project is the cap's decision, not this pass's.
func TestTrimProjectHighlights_RelevantNeverLeavesABareTitle(t *testing.T) {
	doc := projectsDoc(project("P", ProjectLevelRelevant, "Styled the marketing page", "Wrote the changelog"))

	TrimProjectHighlights(doc, VacancyAnalysis{RequiredSkills: []string{"Kafka"}})

	got := StringSliceField(AsSliceOfMaps(CvSections(doc)["projects"])[0], "highlights")
	if len(got) != 1 || got[0] != "Styled the marketing page" {
		t.Errorf("highlights = %v, want the leading bullet kept", got)
	}
}

// Auto never trims away a bullet the vacancy asked about, the same guarantee
// the skills side makes.
func TestTrimProjectHighlights_AutoKeepsEveryVacancyMatch(t *testing.T) {
	doc := projectsDoc(project("P", "",
		"Go service", "Kafka pipeline", "Redis cache", "Postgres schema",
		"Docs", "Changelog", "Logo",
	))
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go", "Kafka", "Redis", "Postgres"}}

	TrimProjectHighlights(doc, analysis)

	got := StringSliceField(AsSliceOfMaps(CvSections(doc)["projects"])[0], "highlights")
	if len(got) != 4 {
		t.Errorf("kept %d bullets, want 4 (every match, past the auto cap)", len(got))
	}
}

// The workspace shows projects as a ranked list where the cap is a selection
// boundary, not a removal: everything stays, the top projectsMax arrive
// selected.
func TestSeedProjectItems_CapSelectsRatherThanDrops(t *testing.T) {
	projects := []map[string]any{
		{"name": "A", "highlights": []any{"x", "y"}},
		{"name": "B", "highlights": []any{"x"}},
		{"name": "C", "highlights": []any{}},
	}

	items := SeedProjectItems(projects, []int{2, 0, 1}, 2)

	if len(items) != 3 {
		t.Fatalf("got %d items, want every project kept", len(items))
	}
	if items[0].SourceText != "C" {
		t.Errorf("first item = %q, want the ranked-first project", items[0].SourceText)
	}
	if !items[0].Selected || !items[1].Selected {
		t.Error("the first two items should arrive selected (projectsMax = 2)")
	}
	if items[2].Selected {
		t.Error("the third item should arrive unselected, not dropped")
	}
	if got := items[1].SourceText; got != "A · 2 bullets" {
		t.Errorf("item text = %q, want the name and its bullet count", got)
	}
}

// A master project name is stored as a markdown link; the workspace shows the
// human half, not the URL.
func TestSeedProjectItems_StripsTheLinkWrapper(t *testing.T) {
	projects := []map[string]any{
		{"name": "[job-finder — AI job search](https://example.com/x)", "highlights": []any{"one"}},
	}

	items := SeedProjectItems(projects, nil, 0)

	if got := items[0].SourceText; got != "job-finder — AI job search · 1 bullet" {
		t.Errorf("item text = %q, want the link text without the URL", got)
	}
}

func TestAssemble_KeepsOnlySelectedProjectsInOrder(t *testing.T) {
	master := projectsDoc(
		project("A", "", "a1"),
		project("B", "", "b1"),
		project("C", "", "c1"),
	)
	idx := func(i int) *int { return &i }
	sections := []Section{{
		Kind: SectionKindProjects, Enabled: true,
		Items: []Item{
			{ID: "1", Origin: OriginProfile, Kind: ItemKindProject, SourceIndex: idx(2), Position: 0, Selected: true},
			{ID: "2", Origin: OriginProfile, Kind: ItemKindProject, SourceIndex: idx(0), Position: 1, Selected: true},
			{ID: "3", Origin: OriginProfile, Kind: ItemKindProject, SourceIndex: idx(1), Position: 2, Selected: false},
		},
	}}

	doc, err := Assemble(master, sections)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if got := projectNames(doc); len(got) != 2 || got[0] != "C" || got[1] != "A" {
		t.Errorf("projects = %v, want [C A] — selected only, in workspace order", got)
	}
}

// A run whose master has no projects gets no projects section: an empty block
// asking to be filled is worse than no block.
func TestSeedFromMaster_NoProjectsSectionWithoutProjects(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{},
		"skills":     []any{},
	}}}

	for _, sec := range SeedFromMaster(master, DefaultShapeConfig()) {
		if sec.Kind == SectionKindProjects {
			t.Fatal("seeded a projects section for a master with no projects")
		}
	}
}

func TestSeedFromMaster_SeedsProjectsSection(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{},
		"skills":     []any{},
		"projects":   []any{map[string]any{"name": "A", "highlights": []any{"a1"}}},
	}}}

	seeded := SeedFromMaster(master, DefaultShapeConfig())
	var found *Section
	for i := range seeded {
		if seeded[i].Kind == SectionKindProjects {
			found = &seeded[i]
		}
	}
	if found == nil {
		t.Fatal("no projects section seeded")
	}
	if len(found.Items) != 1 || !found.Items[0].Selected {
		t.Errorf("items = %+v, want the one project seeded and selected", found.Items)
	}
}
