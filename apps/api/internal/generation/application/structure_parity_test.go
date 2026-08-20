package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func parityMaster() domain.RendercvMaster {
	return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"_order":  []any{"summary", "skills", "experience", "projects", "education", "certifications"},
		"summary": []any{"A summary."},
		"skills": []any{
			map[string]any{"label": "Backend", "details": "Go, Postgres, Docker"},
			map[string]any{"label": "Spoken Languages", "details": "English, Ukrainian"},
		},
		"experience": []any{map[string]any{
			"company":    "Acme",
			"highlights": []any{"Shipped the payments service"},
			"start_date": "2018-01",
			"end_date":   "present",
		}},
		"projects":       []any{map[string]any{"name": "Sidecar", "highlights": []any{"Built a thing"}}},
		"education":      []any{map[string]any{"institution": "MIT", "area": "CS"}},
		"certifications": []any{map[string]any{"name": "AWS SAA"}},
	}}}
}

func TestSplitPipelineLeavesSectionSetAndOrderUnchanged(t *testing.T) {
	master := parityMaster()

	analyze := &stageProvider{name: "generation-analyze", reply: func(string) string {
		return mustJSON(t, domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"})
	}}
	sel := &stageProvider{name: "generation-select", reply: func(string) string {
		return mustJSON(t, domain.TailoredSelection{
			Experience: []domain.TailoredExperience{
				{Company: "Acme", Highlights: []domain.HighlightRef{{SourceIndex: 0}}},
			},
		})
	}}
	years := domain.DeriveTotalExperienceYears(master)
	summary := &stageProvider{name: "generation-summary", reply: summaryReply(t, itoaYears(years)+"+ years of experience in payments.")}
	svc := &Service{llm: GenerationRouters{Analyze: analyze, Select: sel, Premium: sel, Summary: summary, Cover: summary}}

	merged, _, err := svc.tailorRendercvResume(context.Background(), master, domain.VacancyTarget{}, "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{})
	if err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	wantOrder := sectionOrder(t, master)
	gotOrder := sectionOrder(t, merged)
	if !reflect.DeepEqual(wantOrder, gotOrder) {
		t.Errorf("section order changed:\n got %v\nwant %v", gotOrder, wantOrder)
	}

	wantKeys := sectionKeys(master)
	gotKeys := sectionKeys(merged)
	if !reflect.DeepEqual(wantKeys, gotKeys) {
		t.Errorf("section set changed:\n got %v\nwant %v", gotKeys, wantKeys)
	}

	skills := domain.AsSliceOfMaps(domain.CvSections(merged)["skills"])
	if got := domain.StringField(skills[1], "details"); got != "English, Ukrainian" {
		t.Errorf("pinned skill group details = %q, want them untouched", got)
	}
}

func sectionOrder(t *testing.T, doc domain.RendercvMaster) []string {
	t.Helper()
	raw, ok := domain.CvSections(doc)["_order"].([]any)
	if !ok {
		t.Fatal("document has no section order")
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		out = append(out, s.(string))
	}
	return out
}

func sectionKeys(doc domain.RendercvMaster) []string {
	var out []string
	for k := range domain.CvSections(doc) {
		if k != "_order" {
			out = append(out, k)
		}
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
