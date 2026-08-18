package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func rankMaster() domain.RendercvMaster {
	return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{
			map[string]any{
				"company":    "Acme",
				"highlights": []any{"Shipped the payments service", "Cut latency in half", "Mentored two engineers", "Ran the on-call rotation"},
			},
		},
	}}}
}

func rankAnalysis() domain.VacancyAnalysis {
	return domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}
}

func validRankReply(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.RankedSelection{
			Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{2, 0, 3, 1}}},
		})
	}
}

func invalidRankReply(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.RankedSelection{
			Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{0}}},
		})
	}
}

func rankMasterWithSkills() domain.RendercvMaster {
	m := rankMaster()
	domain.CvSections(m)["skills"] = []any{
		map[string]any{"label": "Languages", "details": "Go, TypeScript"},
		map[string]any{"label": "Cloud", "details": "AWS, GCP"},
		map[string]any{"label": "Data", "details": "Postgres, Redis"},
	}
	return m
}

func TestSkillGroupOrderIsVerifiedLikeAnAchievementRanking(t *testing.T) {
	reply := func(order []int) func(string) string {
		return func(string) string {
			return mustJSON(t, domain.RankedSelection{
				Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{2, 0, 3, 1}}},
				Skills:     domain.RankedSkills{GroupOrder: order},
			})
		}
	}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

	t.Run("verified order is returned", func(t *testing.T) {
		provider := &stageProvider{name: "generation-select", reply: reply([]int{2, 0, 1})}

		_, order := rankExperienceSections(context.Background(), provider, "", rankMasterWithSkills(), rankAnalysis(), cfg)

		if want := []int{2, 0, 1}; len(order) != len(want) {
			t.Fatalf("order = %v, want %v", order, want)
		}
		if provider.calls != 1 {
			t.Errorf("provider called %d times, want 1", provider.calls)
		}
	})

	t.Run("order omitting a group is retried then dropped", func(t *testing.T) {
		provider := &stageProvider{name: "generation-select", reply: reply([]int{0, 1})}

		_, order := rankExperienceSections(context.Background(), provider, "", rankMasterWithSkills(), rankAnalysis(), cfg)

		if order != nil {
			t.Errorf("order = %v, want nil so the master-order seed stands", order)
		}
		if provider.calls != 2 {
			t.Errorf("provider called %d times, want 2 (the bad groupOrder alone must trigger the retry)", provider.calls)
		}
	})
}

func TestTwiceRejectedRankingFallsBackToMasterOrder(t *testing.T) {
	provider := &stageProvider{name: "generation-select", reply: invalidRankReply(t)}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

	results, _ := rankExperienceSections(context.Background(), provider, "", rankMaster(), rankAnalysis(), cfg)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	r := results[0]
	if r.entryKey != "Acme" {
		t.Errorf("entryKey = %q, want Acme", r.entryKey)
	}
	if !r.fallbackUsed {
		t.Error("fallbackUsed = false, want true after two rejected attempts")
	}
	if r.items != nil {
		t.Errorf("items = %+v, want nil (fallback leaves the master-order seed untouched)", r.items)
	}

	if provider.calls != 2 {
		t.Errorf("provider called %d times, want 2 (one attempt, one retry)", provider.calls)
	}
}

func TestValidRankingOnFirstAttemptNeedsNoRetry(t *testing.T) {
	provider := &stageProvider{name: "generation-select", reply: validRankReply(t)}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

	results, _ := rankExperienceSections(context.Background(), provider, "", rankMaster(), rankAnalysis(), cfg)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	r := results[0]
	if r.fallbackUsed {
		t.Error("fallbackUsed = true, want false for a ranking that verified on the first attempt")
	}
	if r.items == nil {
		t.Fatal("items = nil, want the ranked items")
	}
	if len(r.items) != 4 {
		t.Fatalf("len(items) = %d, want 4 (every master bullet accounted for)", len(r.items))
	}
	if provider.calls != 1 {
		t.Errorf("provider called %d times, want 1 (no retry needed)", provider.calls)
	}

	wantOrder := []int{2, 0, 3, 1}
	for i, it := range r.items {
		if it.SourceIndex == nil || *it.SourceIndex != wantOrder[i] {
			t.Errorf("items[%d].SourceIndex = %v, want %d", i, it.SourceIndex, wantOrder[i])
		}
	}
	if !r.items[0].Selected || !r.items[1].Selected {
		t.Error("the top N ranked items must be selected")
	}
	if r.items[2].Selected || r.items[3].Selected {
		t.Error("ranked items beyond N must be unselected, not dropped")
	}
}

func TestRankingValidOnRetryIsUsed(t *testing.T) {
	attempt := 0
	provider := &stageProvider{name: "generation-select", reply: func(string) string {
		attempt++
		if attempt == 1 {
			return invalidRankReply(t)("")
		}
		return validRankReply(t)("")
	}}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

	results, _ := rankExperienceSections(context.Background(), provider, "", rankMaster(), rankAnalysis(), cfg)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	r := results[0]
	if r.fallbackUsed {
		t.Error("fallbackUsed = true, want false — the retry produced a valid ranking")
	}
	if r.items == nil || len(r.items) != 4 {
		t.Fatalf("items = %+v, want 4 ranked items from the retry", r.items)
	}
	if provider.calls != 2 {
		t.Errorf("provider called %d times, want 2", provider.calls)
	}
}
