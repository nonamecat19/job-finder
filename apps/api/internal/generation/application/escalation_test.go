package application

import (
	"context"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

// escalationMaster has a skill group the vacancy will require, so a truncated
// selection response is detectable as a shortfall rather than a stylistic
// difference.
func escalationMaster() domain.RendercvMaster {
	return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"summary": []any{"A summary."},
		"skills": []any{
			map[string]any{"label": "Backend", "details": "Go, Postgres, Docker"},
			map[string]any{"label": "Frontend", "details": "React, TypeScript"},
		},
		"experience": []any{map[string]any{
			"company":    "Acme",
			"highlights": []any{"Shipped the payments service", "Cut latency in half"},
			"start_date": "2018-01",
			"end_date":   "present",
		}},
	}}}
}

func truncatedSelection(t *testing.T) func(string) string {
	// Returns both groups but guts the Backend one, dropping Docker — which the
	// vacancy requires. Note the shape of this fixture: omitting a group
	// entirely would NOT be a shortfall, because the merge keys skills by index
	// and leaves an unmentioned group at its master value. Only content the
	// model returns in truncated form actually reaches the document, and that is
	// exactly what the cheap models were measured doing (glm-4.7-flash returned
	// 17 skill tokens where the master had 187).
	return func(string) string {
		return mustJSON(t, domain.TailoredSelection{
			Skills: []domain.TailoredSkillGroup{
				{Index: 0, Details: "Go"},
				{Index: 1, Details: "React, TypeScript"},
			},
			Experience: []domain.TailoredExperience{
				{Company: "Acme", Highlights: []string{"Shipped the payments service"}},
			},
		})
	}
}

func completeSelection(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.TailoredSelection{
			Skills: []domain.TailoredSkillGroup{
				{Index: 0, Details: "Go, Postgres, Docker"},
				{Index: 1, Details: "React, TypeScript"},
			},
			Experience: []domain.TailoredExperience{
				{Company: "Acme", Highlights: []string{"Shipped the payments service", "Cut latency in half"}},
			},
		})
	}
}

func escalationService(t *testing.T, economy, premium func(string) string) (*Service, *stageProvider, *stageProvider) {
	t.Helper()
	analyze := &stageProvider{name: "generation-analyze", reply: func(string) string {
		return mustJSON(t, domain.VacancyAnalysis{
			RequiredSkills:   []string{"Go", "Docker"},
			NiceToHaveSkills: []string{"React"},
			ExperienceLevel:  "senior",
		})
	}}
	sel := &stageProvider{name: "generation-select", reply: economy}
	prem := &stageProvider{name: "generation-select-premium", reply: premium}
	years := domain.DeriveTotalExperienceYears(escalationMaster())
	summary := &stageProvider{name: "generation-summary", reply: summaryReply(t, itoaYears(years)+"+ years of experience in payments.")}
	svc := &Service{llm: GenerationRouters{Analyze: analyze, Select: sel, Premium: prem, Summary: summary, Cover: summary}}
	return svc, sel, prem
}

func itoaYears(y int) string {
	if y == 0 {
		return "0"
	}
	var digits []byte
	for y > 0 {
		digits = append([]byte{byte('0' + y%10)}, digits...)
		y /= 10
	}
	return string(digits)
}

// 035 FR-007 / US2: two consecutive shortfalls escalate to the premium model,
// the run completes, and the escalation is recorded.
func TestSelectionEscalatesAfterRepeatedShortfall(t *testing.T) {
	svc, sel, prem := escalationService(t, truncatedSelection(t), completeSelection(t))
	prov := &runProvenance{}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 1

	_, _, err := svc.tailorRendercvResume(context.Background(), escalationMaster(), "Go role", domain.GroundingModerate, cfg, nil, nil, prov)
	if err != nil {
		t.Fatalf("run should complete via escalation, got: %v", err)
	}

	if sel.calls != selectionAttempts-1 {
		t.Errorf("economy selection called %d times, want %d before escalating", sel.calls, selectionAttempts-1)
	}
	if prem.calls != 1 {
		t.Errorf("premium selection called %d times, want exactly 1", prem.calls)
	}
	if !prov.selectionEscalated() {
		t.Error("escalation not recorded on the run; an operator could not tell this run cost more")
	}
}

// A healthy economy response must not retry and must never reach the premium
// model — the escalation ladder is for repeated shortfalls only.
func TestHealthySelectionNeverEscalates(t *testing.T) {
	svc, sel, prem := escalationService(t, completeSelection(t), completeSelection(t))
	prov := &runProvenance{}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 1

	if _, _, err := svc.tailorRendercvResume(context.Background(), escalationMaster(), "Go role", domain.GroundingModerate, cfg, nil, nil, prov); err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	if sel.calls != 1 {
		t.Errorf("economy selection called %d times on healthy output, want 1", sel.calls)
	}
	if prem.calls != 0 {
		t.Errorf("premium selection called %d times on healthy output, want 0", prem.calls)
	}
	if prov.selectionEscalated() {
		t.Error("run marked as escalated despite no shortfall")
	}
}

// When even the premium model returns incomplete output, the run fails rather
// than rendering a hollowed-out resume — that is the whole point of US2.
func TestPersistentShortfallFailsRatherThanRendering(t *testing.T) {
	svc, _, _ := escalationService(t, truncatedSelection(t), truncatedSelection(t))
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 1

	_, _, err := svc.tailorRendercvResume(context.Background(), escalationMaster(), "Go role", domain.GroundingModerate, cfg, nil, nil, &runProvenance{})
	if err == nil {
		t.Fatal("run succeeded on persistently incomplete selection; a truncated resume must never render")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error = %v, want it to name the completeness shortfall", err)
	}
}
