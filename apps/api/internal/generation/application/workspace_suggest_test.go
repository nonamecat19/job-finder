package application

import (
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func suggestMaster() domain.RendercvMaster {
	return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{
			map[string]any{
				"company":    "Acme",
				"highlights": []any{"Shipped the payments service"},
			},
		},
		"skills": []any{
			map[string]any{"label": "Languages", "details": "Go, Python"},
		},
	}}}
}

func TestBuildSuggestionItemsAreUnselected(t *testing.T) {
	master := suggestMaster()
	suggestions := domain.SuggestionSet{
		Experience: []domain.ExperienceSuggestions{
			{Company: "Acme", Bullets: []string{"Led the migration to Kubernetes", "Cut infra spend by a third"}},
		},
		Skills: []string{"Kubernetes", "Terraform"},
	}

	experience, skills := buildSuggestionItems(suggestions, master)

	acmeItems, ok := experience["Acme"]
	if !ok || len(acmeItems) != 2 {
		t.Fatalf("experience[Acme] = %+v, want 2 items", acmeItems)
	}
	for _, it := range acmeItems {
		if it.Selected {
			t.Errorf("experience item %q: Selected = true, want false", it.SourceText)
		}
		if it.Origin != domain.OriginAI {
			t.Errorf("experience item %q: Origin = %q, want %q", it.SourceText, it.Origin, domain.OriginAI)
		}
	}

	if len(skills) != 2 {
		t.Fatalf("skills = %+v, want 2 items", skills)
	}
	for _, it := range skills {
		if it.Selected {
			t.Errorf("skill item %q: Selected = true, want false", it.SourceText)
		}
		if it.Origin != domain.OriginAI {
			t.Errorf("skill item %q: Origin = %q, want %q", it.SourceText, it.Origin, domain.OriginAI)
		}
	}
}

func TestBuildSuggestionItemsDropsUnknownCompany(t *testing.T) {
	master := suggestMaster()
	suggestions := domain.SuggestionSet{
		Experience: []domain.ExperienceSuggestions{
			{Company: "Initech", Bullets: []string{"Did something at a company that isn't in the master"}},
		},
	}

	experience, _ := buildSuggestionItems(suggestions, master)

	if _, ok := experience["Initech"]; ok {
		t.Fatalf("experience[Initech] present, want the unmatched-company entry dropped")
	}
	if len(experience) != 0 {
		t.Fatalf("experience = %+v, want empty", experience)
	}
}

func TestBuildSuggestionItemsSuppressesDuplicate(t *testing.T) {
	master := suggestMaster()
	suggestions := domain.SuggestionSet{
		Experience: []domain.ExperienceSuggestions{
			{Company: "Acme", Bullets: []string{"shipped the payments service"}},
		},
	}

	experience, _ := buildSuggestionItems(suggestions, master)

	if items, ok := experience["Acme"]; ok && len(items) != 0 {
		t.Fatalf("experience[Acme] = %+v, want the duplicate suppressed", items)
	}
}
