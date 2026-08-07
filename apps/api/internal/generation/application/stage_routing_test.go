package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

// stageProvider records what a stage asked for and replies with a canned
// payload, so a test can assert routing and prompt content without a gateway.
type stageProvider struct {
	name     string
	model    string
	prompts  []string
	reply    func(prompt string) string
	served   string
	usage    *llm.Usage
	calls    int
	replyErr error
}

func (p *stageProvider) ModelName() string { return p.name }

func (p *stageProvider) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return p.CompleteJSON(ctx, prompt, opts)
}

func (p *stageProvider) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	p.calls++
	p.prompts = append(p.prompts, prompt)
	if opts != nil {
		p.model = opts.Model
	}
	if p.replyErr != nil {
		return "", p.replyErr
	}
	if p.served != "" {
		llm.ReportServedModel(ctx, p.served)
	}
	if p.usage != nil {
		llm.ReportUsage(ctx, *p.usage)
	}
	return p.reply(prompt), nil
}

// CompleteChat satisfies the 037 Provider interface. The fake's behaviour lives
// in CompleteJSON, so this delegates to it with the final turn as the prompt —
// which is what the real adapters do in reverse. Tool calls are never
// fabricated here: a fake that invented one would make a tool-loop test pass
// for the wrong reason.
func (p *stageProvider) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	text, err := p.CompleteJSON(ctx, prompt, opts)
	return llm.ChatResult{Content: text}, err
}

func (p *stageProvider) Embed(context.Context, string) ([]float32, error) { return nil, nil }

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func analysisReply(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"})
	}
}

func selectionReply(t *testing.T) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.TailoredSelection{Experience: []domain.TailoredExperience{
			{Company: "Acme", Highlights: []string{"Did a thing"}},
		}})
	}
}

func summaryReply(t *testing.T, text string) func(string) string {
	return func(string) string {
		return mustJSON(t, domain.TailoredSummary{Summary: text})
	}
}

func stagedService(t *testing.T) (*Service, *stageProvider, *stageProvider, *stageProvider, *stageProvider) {
	t.Helper()
	analyze := &stageProvider{name: "generation-analyze", reply: analysisReply(t)}
	sel := &stageProvider{name: "generation-select", reply: selectionReply(t)}
	premium := &stageProvider{name: "generation-select-premium", reply: selectionReply(t)}
	// The years figure has to match what the master's dates derive to, or the
	// structure verifier correctly strips it and the assertions below would be
	// testing the stripped text rather than the summary stage's output.
	years := domain.DeriveTotalExperienceYears(stageMaster())
	summary := &stageProvider{name: "generation-summary", reply: summaryReply(t, fmt.Sprintf("%d+ years of experience building Go services.", years))}
	svc := &Service{llm: GenerationRouters{Analyze: analyze, Select: sel, Premium: premium, Summary: summary, Cover: summary}}
	return svc, analyze, sel, premium, summary
}

func stageMaster() domain.RendercvMaster {
	return domain.RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"summary": []any{"A summary."},
		"skills":  []any{map[string]any{"label": "Backend", "details": "Go, Postgres"}},
		"experience": []any{map[string]any{
			"company":    "Acme",
			"highlights": []any{"Did a thing"},
			"start_date": "2018-01",
			"end_date":   "present",
		}},
	}}}
}

// 035 FR-001/FR-002/FR-003: each stage goes to its own provider, and none of
// them is told which model to use — the task key is the whole request.
func TestEachStageRoutesToItsOwnProvider(t *testing.T) {
	svc, analyze, sel, premium, summary := stagedService(t)

	merged, _, err := svc.tailorRendercvResume(context.Background(), stageMaster(), "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{})
	if err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	if analyze.calls != 1 || sel.calls != 1 || summary.calls != 1 {
		t.Errorf("calls: analyze=%d select=%d summary=%d, want 1 each", analyze.calls, sel.calls, summary.calls)
	}
	if premium.calls != 0 {
		t.Errorf("premium provider called %d times on a healthy run, want 0 — escalation is for repeated shortfalls only", premium.calls)
	}
	for _, p := range []*stageProvider{analyze, sel, summary} {
		if p.model != "" {
			t.Errorf("%s was told to use model %q; the application must send a task key and nothing else", p.name, p.model)
		}
	}

	sections := domain.CvSections(merged)
	got := sections["summary"].([]any)[0].(string)
	if !strings.Contains(got, "years of experience building Go services") {
		t.Errorf("merged summary = %q, want the summary stage's output", got)
	}
}

// 035 FR-004: the premium stage's prompt carries the brief, not the master.
func TestSummaryPromptExcludesMasterProfile(t *testing.T) {
	svc, _, _, _, summary := stagedService(t)

	if _, _, err := svc.tailorRendercvResume(context.Background(), stageMaster(), "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{}); err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	prompt := summary.prompts[0]
	if strings.Contains(prompt, "start_date") || strings.Contains(prompt, "Postgres, ") {
		t.Errorf("summary prompt carries master profile detail it does not need:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Did a thing") {
		t.Errorf("summary prompt is missing the selected achievements it must reference:\n%s", prompt)
	}
	if !strings.Contains(prompt, "years of experience") {
		t.Errorf("summary prompt is missing the derived years figure:\n%s", prompt)
	}
}

// 035 FR-012: a summary served by a fallback is recorded as substituted, and a
// summary served by tier 1 is not. The signal is the proxy's own
// attempted-fallbacks count, so the application never learns which upstream
// model was configured — only that the chain advanced.
func TestSummarySubstitutionIsRecorded(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fallbacks   int
		servedModel string
		wantSubbed  bool
	}{
		{name: "tier 1 served", fallbacks: 0, servedModel: "anthropic/claude-sonnet-5", wantSubbed: false},
		{name: "fallback served", fallbacks: 1, servedModel: "cerebras/gpt-oss-120b", wantSubbed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, _, summary := stagedService(t)
			summary.served = tc.servedModel
			summary.usage = &llm.Usage{
				CostUSD:            0.008,
				ServedGroup:        "generation-summary",
				AttemptedFallbacks: tc.fallbacks,
				Substituted:        tc.fallbacks > 0,
			}
			prov := &runProvenance{}

			if _, _, err := svc.tailorRendercvResume(context.Background(), stageMaster(), "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, prov); err != nil {
				t.Fatalf("tailorRendercvResume: %v", err)
			}
			if got := prov.summarySubstituted(); got != tc.wantSubbed {
				t.Errorf("summarySubstituted = %v, want %v", got, tc.wantSubbed)
			}
			if got := prov.summaryModel(); got == nil || *got != tc.servedModel {
				t.Errorf("summaryModel = %v, want %q", got, tc.servedModel)
			}
			if prov.totalCostUSD() <= 0 {
				t.Error("run recorded no cost; FR-017 requires measured economics per stage")
			}
		})
	}
}

// 035 FR-011/SC-008: with no gateway configured every stage still resolves —
// the routers fall back to the local provider and a resume is produced.
func TestLocalOnlyRunProducesAResume(t *testing.T) {
	local := &stageProvider{name: "local", reply: func(prompt string) string {
		switch {
		case strings.Contains(prompt, "Analyze this job vacancy"):
			return mustJSON(t, domain.VacancyAnalysis{RequiredSkills: []string{"Go"}})
		case strings.Contains(prompt, "Write the professional summary"):
			return mustJSON(t, domain.TailoredSummary{Summary: fmt.Sprintf("%d+ years of experience.", domain.DeriveTotalExperienceYears(stageMaster()))})
		default:
			return mustJSON(t, domain.TailoredSelection{Experience: []domain.TailoredExperience{
				{Company: "Acme", Highlights: []string{"Did a thing"}},
			}})
		}
	}}
	// Every stage routes to the same local provider, which is what
	// llm.NewRouter does when GATEWAY_URL is empty.
	svc := &Service{llm: GenerationRouters{Analyze: local, Select: local, Premium: local, Summary: local, Cover: local}}

	merged, _, err := svc.tailorRendercvResume(context.Background(), stageMaster(), "Go role", domain.GroundingModerate, domain.DefaultShapeConfig(), nil, nil, &runProvenance{})
	if err != nil {
		t.Fatalf("local-only run failed: %v", err)
	}
	if domain.CvSections(merged) == nil {
		t.Fatal("local-only run produced no document")
	}
	if local.calls < 3 {
		t.Errorf("local provider served %d calls, want at least one per stage", local.calls)
	}
}
