package application

import (
	"context"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

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

	return func(string) string {
		return mustJSON(t, domain.TailoredSelection{
			Experience: []domain.TailoredExperience{
				{Company: "Acme", Highlights: []domain.HighlightRef{{SourceIndex: 0}}},
			},
		})
	}
}

func completeSelection(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.TailoredSelection{
			Experience: []domain.TailoredExperience{
				{Company: "Acme", Highlights: []domain.HighlightRef{{SourceIndex: 0}, {SourceIndex: 1}}},
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

func TestSelectionEscalatesAfterRepeatedShortfall(t *testing.T) {
	svc, sel, prem := escalationService(t, truncatedSelection(t), completeSelection(t))
	prov := &runProvenance{}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

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

func TestHealthySelectionNeverEscalates(t *testing.T) {
	svc, sel, prem := escalationService(t, completeSelection(t), completeSelection(t))
	prov := &runProvenance{}
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

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

func TestPersistentShortfallFailsRatherThanRendering(t *testing.T) {
	svc, _, _ := escalationService(t, truncatedSelection(t), truncatedSelection(t))
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2

	_, _, err := svc.tailorRendercvResume(context.Background(), escalationMaster(), "Go role", domain.GroundingModerate, cfg, nil, nil, &runProvenance{})
	if err == nil {
		t.Fatal("run succeeded on persistently incomplete selection; a truncated resume must never render")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error = %v, want it to name the completeness shortfall", err)
	}
}
