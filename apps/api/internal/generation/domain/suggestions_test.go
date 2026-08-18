package domain

import "testing"

func suggestionsMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{
			map[string]any{
				"company": "Acme",
				"highlights": []any{
					"Led a team of five engineers to launch the new payments platform",
				},
			},
		},
		"skills": []any{
			map[string]any{"label": "Languages", "details": "Go, Python, TypeScript"},
		},
	}}}
}

func TestSuppressDuplicateSuggestionsRemovesNormalisedMatch(t *testing.T) {
	master := suggestionsMaster()
	suggestions := SuggestionSet{
		Experience: []ExperienceSuggestions{
			{Company: "Acme", Bullets: []string{"led a team of five engineers to launch the new payments platform"}},
		},
	}

	out := SuppressDuplicateSuggestions(suggestions, master)

	if got := len(out.Experience[0].Bullets); got != 0 {
		t.Fatalf("Bullets = %v, want none (exact normalised match to a master bullet)", out.Experience[0].Bullets)
	}
}

func TestSuppressDuplicateSuggestionsRemovesHighContainment(t *testing.T) {
	master := suggestionsMaster()
	suggestions := SuggestionSet{
		Experience: []ExperienceSuggestions{

			{Company: "Acme", Bullets: []string{"Led a team of five engineers to launch the payments platform"}},
		},
	}

	out := SuppressDuplicateSuggestions(suggestions, master)

	if got := len(out.Experience[0].Bullets); got != 0 {
		t.Fatalf("Bullets = %v, want none (>= 0.9 word-set containment against a master bullet)", out.Experience[0].Bullets)
	}
}

func TestSuppressDuplicateSuggestionsKeepsDistinctBullet(t *testing.T) {
	master := suggestionsMaster()
	suggestions := SuggestionSet{
		Experience: []ExperienceSuggestions{
			{Company: "Acme", Bullets: []string{"Designed the disaster recovery runbook adopted company-wide"}},
		},
		Skills: []string{"Kubernetes"},
	}

	out := SuppressDuplicateSuggestions(suggestions, master)

	if got := out.Experience[0].Bullets; len(got) != 1 || got[0] != "Designed the disaster recovery runbook adopted company-wide" {
		t.Fatalf("Bullets = %v, want the distinct bullet kept untouched", got)
	}
	if got := out.Skills; len(got) != 1 || got[0] != "Kubernetes" {
		t.Fatalf("Skills = %v, want the non-duplicate skill kept", got)
	}
}

func TestSuppressDuplicateSuggestionsRemovesDuplicateSkillToken(t *testing.T) {
	master := suggestionsMaster()
	suggestions := SuggestionSet{Skills: []string{"Go", "Rust"}}

	out := SuppressDuplicateSuggestions(suggestions, master)

	if len(out.Skills) != 1 || out.Skills[0] != "Rust" {
		t.Fatalf("Skills = %v, want only the non-duplicate skill kept", out.Skills)
	}
}

func TestSuppressDuplicateSuggestionsLeavesMasterUntouched(t *testing.T) {
	master := suggestionsMaster()
	before := StringSliceField(AsSliceOfMaps(CvSections(master)["experience"])[0], "highlights")
	suggestions := SuggestionSet{
		Experience: []ExperienceSuggestions{
			{Company: "Acme", Bullets: []string{"led a team of five engineers to launch the new payments platform"}},
		},
	}

	SuppressDuplicateSuggestions(suggestions, master)

	after := StringSliceField(AsSliceOfMaps(CvSections(master)["experience"])[0], "highlights")
	if len(before) != len(after) || before[0] != after[0] {
		t.Fatalf("master highlights changed: before=%v after=%v", before, after)
	}
}
