package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

// rankMaster is a small master with two experience entries, sized so the
// tests below can distinguish "ranking accepted" from "fell back to master
// order" by inspecting the returned indices.
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

// A valid ranking reply for rankMaster's single "Acme" entry. cfg.ExperienceBulletsMin
// is set to 2 in these tests, so K = min(2*2, 4) = 4 — every index, in some order.
func validRankReply(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.RankedSelection{
			Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{2, 0, 3, 1}}},
		})
	}
}

// An invalid ranking reply: too short (K=4, only 1 returned) — VerifyRanking
// rejects it on every attempt.
func invalidRankReply(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.RankedSelection{
			Experience: []domain.RankedExperience{{Company: "Acme", Ranking: []int{0}}},
		})
	}
}

// rankMasterWithSkills adds three skill groups to rankMaster, so the skills
// half of the same response has something to order.
func rankMasterWithSkills() domain.RendercvMaster {
	m := rankMaster()
	domain.CvSections(m)["skills"] = []any{
		map[string]any{"label": "Languages", "details": "Go, TypeScript"},
		map[string]any{"label": "Cloud", "details": "AWS, GCP"},
		map[string]any{"label": "Data", "details": "Postgres, Redis"},
	}
	return m
}

// T059/T060: a verified groupOrder comes back from the same call that ranked
// the achievements, and an unverifiable one is retried once and then dropped
// so the caller keeps the master-order seed.
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

// T038: a ranking rejected on both attempts falls back to master order
// (fallbackUsed = true, no items to persist — the caller leaves the
// StartGenerationRun seed exactly as it is) rather than failing the run.
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
	// The stage must have been given exactly two chances, matching the
	// existing groundingAttempts idiom used elsewhere in this pipeline.
	if provider.calls != 2 {
		t.Errorf("provider called %d times, want 2 (one attempt, one retry)", provider.calls)
	}
}

// A ranking that verifies on the first attempt is used as-is, with no retry
// and fallbackUsed left false.
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
	// Ranked order 2,0,3,1 with N=2 selected: first two selected.
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

// A ranking invalid on the first attempt but valid on the retry is used —
// the retry response, not a fallback.
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
