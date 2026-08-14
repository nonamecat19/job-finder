package domain

import "testing"

func rankingMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills": []any{
			map[string]any{"label": "Backend", "details": "Postgres, Go, Redis, Docker"},
			map[string]any{"label": "Frontend", "details": "React, TypeScript"},
			map[string]any{"label": "Spoken Languages", "details": "Ukrainian (native), English (B2)"},
		},
	}}}
}

func rankingAnalysis() VacancyAnalysis {
	return VacancyAnalysis{RequiredSkills: []string{"Go", "Docker"}, NiceToHaveSkills: []string{"Redis"}}
}

func skillDetails(t *testing.T, doc RendercvMaster) []string {
	t.Helper()
	var out []string
	for _, g := range AsSliceOfMaps(CvSections(doc)["skills"]) {
		out = append(out, StringField(g, "details"))
	}
	return out
}

func TestRankSkillsPutsRequiredSkillsFirst(t *testing.T) {
	doc := rankingMaster()
	RankSkills(doc, rankingAnalysis(), DefaultShapeConfig())

	if got := skillDetails(t, doc)[0]; got != "Go, Docker, Redis, Postgres" {
		t.Errorf("Backend details = %q, want required skills first, then nice-to-have, then the rest in master order", got)
	}
}

// The whole point of moving this off the model: an ordering pass cannot lose a
// skill, because its output is a permutation of its input.
func TestRankSkillsDropsNothing(t *testing.T) {
	doc := rankingMaster()
	before := MasterSkillTokens(doc)
	RankSkills(doc, rankingAnalysis(), DefaultShapeConfig())
	after := MasterSkillTokens(doc)

	if len(before) != len(after) {
		t.Fatalf("token count changed: %d before, %d after", len(before), len(after))
	}
	for tok := range before {
		if !after[tok] {
			t.Errorf("skill %q disappeared during ranking", tok)
		}
	}
}

func TestRankSkillsLeavesPinnedGroupVerbatim(t *testing.T) {
	doc := rankingMaster()
	RankSkills(doc, VacancyAnalysis{RequiredSkills: []string{"English"}}, DefaultShapeConfig())

	if got := skillDetails(t, doc)[2]; got != "Ukrainian (native), English (B2)" {
		t.Errorf("spoken languages reordered to %q; it is a fact about the candidate, not a tailoring target", got)
	}
}

// Group order is the master's authored order unless a cap forces a choice.
func TestRankSkillsKeepsGroupOrderWithoutACap(t *testing.T) {
	doc := rankingMaster()
	RankSkills(doc, VacancyAnalysis{RequiredSkills: []string{"React"}}, DefaultShapeConfig())

	groups := AsSliceOfMaps(CvSections(doc)["skills"])
	if got := StringField(groups[0], "label"); got != "Backend" {
		t.Errorf("first group = %q, want the master's order preserved when every group renders", got)
	}
}

func TestRankSkillsOrdersGroupsByRelevanceUnderACap(t *testing.T) {
	doc := rankingMaster()
	cfg := DefaultShapeConfig()
	cfg.SkillsMaxGroups = 2
	RankSkills(doc, VacancyAnalysis{RequiredSkills: []string{"React", "TypeScript"}}, cfg)

	groups := AsSliceOfMaps(CvSections(doc)["skills"])
	if got := StringField(groups[0], "label"); got != "Frontend" {
		t.Errorf("first group = %q, want the group the vacancy actually asks for", got)
	}
	// The cap that follows keeps pinned groups regardless of position, so
	// ranking must not spend a slot on them.
	if got := StringField(groups[len(groups)-1], "label"); got != "Spoken Languages" {
		t.Errorf("last group = %q, want the pinned group sorted out of the contested slots", got)
	}
}

// Same inputs, same output — the property that made this worth taking off a
// sampling model in the first place.
func TestRankSkillsIsDeterministic(t *testing.T) {
	first, second := rankingMaster(), rankingMaster()
	RankSkills(first, rankingAnalysis(), DefaultShapeConfig())
	RankSkills(second, rankingAnalysis(), DefaultShapeConfig())

	a, b := skillDetails(t, first), skillDetails(t, second)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("group %d differs between runs: %q vs %q", i, a[i], b[i])
		}
	}
}

// Vacancy relevance is coarse — required, nice-to-have, or nothing — so most
// entries in a group tie. The profile breaks those ties: a skill the candidate
// actually wrote about outranks one that only appears in the skills list.
func TestRankSkillsBreaksTiesOnProfileEvidence(t *testing.T) {
	doc := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills": []any{
			map[string]any{"label": "Backend", "details": "Elixir, Kafka, Haskell"},
		},
		"experience": []any{
			map[string]any{
				"company":    "Acme",
				"highlights": []any{"Built the Kafka ingest pipeline", "Tuned Kafka consumer lag"},
			},
		},
	}}}

	RankSkills(doc, VacancyAnalysis{}, DefaultShapeConfig())

	if got := skillDetails(t, doc)[0]; got != "Kafka, Elixir, Haskell" {
		t.Errorf("details = %q, want the evidenced skill first", got)
	}
}

// Evidence is a tiebreaker, never an override: a required skill the rest of
// the profile never mentions still outranks a well-evidenced skill the vacancy
// did not ask for.
func TestRankSkillsEvidenceNeverOutranksTheVacancy(t *testing.T) {
	doc := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills": []any{
			map[string]any{"label": "Backend", "details": "Kafka, Elixir"},
		},
		"experience": []any{
			map[string]any{
				"company":    "Acme",
				"highlights": []any{"Built the Kafka ingest pipeline", "Tuned Kafka consumer lag"},
			},
		},
	}}}

	RankSkills(doc, VacancyAnalysis{RequiredSkills: []string{"Elixir"}}, DefaultShapeConfig())

	if got := skillDetails(t, doc)[0]; got != "Elixir, Kafka" {
		t.Errorf("details = %q, want the required skill first", got)
	}
}

// Summing relevance lets two nice-to-haves outweigh one hard requirement, and
// under a group cap that costs a required skill its place on the page.
func TestRankGroupsPrefersRequiredCoverageOverSummedScore(t *testing.T) {
	doc := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills": []any{
			map[string]any{"label": "Tooling", "details": "Redis, Docker"},
			map[string]any{"label": "Backend", "details": "Go, Elixir"},
		},
	}}}
	cfg := DefaultShapeConfig()
	cfg.SkillsMaxGroups = 1

	RankSkills(doc, VacancyAnalysis{
		RequiredSkills:   []string{"Go"},
		NiceToHaveSkills: []string{"Redis", "Docker"},
	}, cfg)

	groups := AsSliceOfMaps(CvSections(doc)["skills"])
	if got := StringField(groups[0], "label"); got != "Backend" {
		t.Errorf("first group = %q, want the group carrying the required skill", got)
	}
}
