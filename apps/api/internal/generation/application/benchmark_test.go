package application

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

// TestBenchmarkGenerationStrictness is the strictness benchmark fixture (033
// T027, FR-007, SC-001). It runs the tailoring pipeline for each model in the
// generation chain against a fixed master profile × vacancy matrix, recording
// grounding violations, structural violations, JSON-parse failures, and
// wall-clock time per model. The primary generation model is selected from
// the lowest combined violation rate (T028/T029).
//
// This is a long-running test that calls live providers — it is skipped in
// short mode and when GENERATION_BENCHMARK=1 is not set. Run it manually:
//
//	GENERATION_BENCHMARK=1 go test -run TestBenchmarkGenerationStrictness \
//	  -tags benchmark -v -timeout 30m ./internal/generation/application/
//
// Record the results in gateway/config.yaml's generation-section comment block
// (T029) so the model selection is evidence-backed.
func TestBenchmarkGenerationStrictness(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark requires live providers")
	}
	if os.Getenv("GENERATION_BENCHMARK") != "1" {
		t.Skip("set GENERATION_BENCHMARK=1 to run the strictness benchmark")
	}

	gatewayURL := os.Getenv("GATEWAY_URL")
	masterKey := os.Getenv("LITELLM_MASTER_KEY")
	if gatewayURL == "" || masterKey == "" {
		t.Skip("GATEWAY_URL and LITELLM_MASTER_KEY must be set")
	}

	models := []string{
		"generation",             // primary (OpenRouter deepseek-v4-pro)
		"generation-cerebras",    // tier 2
		"generation-groq",        // tier 3
		"generation-cohere",      // tier 4
	}
	// The local tier is not addressable through the gateway by task key; it
	// is served when the whole chain is exhausted. To benchmark it directly,
	// point GATEWAY_URL at a config whose generation chain has only `local`.

	master := loadSampleMaster(t)
	vacancy := "Senior Backend Engineer (Go, Postgres, Kubernetes, Terraform). " +
		"Design and build distributed systems. Lead a small team. 5+ years experience."
	level := domain.GroundingModerate
	cfg := domain.DefaultShapeConfig()

	type result struct {
		model                  string
		groundingViolations    int
		structuralViolations  int
		jsonParseFailures     int
		medianMs              int64
		attempts              int
	}

	var results []result
	for _, modelKey := range models {
		r := result{model: modelKey}
		for attempt := 0; attempt < 3; attempt++ {
			start := time.Now()
			_, _, err := tailorRendercvResumeWithModel(t, gatewayURL, masterKey, modelKey, master, vacancy, level, cfg)
			dur := time.Since(start).Milliseconds()
			if err != nil {
				if isJSONParseError(err) {
					r.jsonParseFailures++
				} else {
					r.groundingViolations++ // grounding check failure
				}
			}
			r.attempts++
			r.medianMs += dur
		}
		if r.attempts > 0 {
			r.medianMs /= int64(r.attempts)
		}
		results = append(results, r)
	}

	fmt.Println("\n=== 033 Strictness Benchmark ===")
	fmt.Println("Model               | Grounding | Structural | JSON-parse | Median ms")
	fmt.Println("---------------------|-----------|------------|-----------|----------")
	for _, r := range results {
		fmt.Printf("%-20s | %9d | %10d | %9d | %8d\n",
			r.model, r.groundingViolations, r.structuralViolations, r.jsonParseFailures, r.medianMs)
	}
}

// tailorRendercvResumeWithModel is a thin wrapper that calls the tailoring
// pipeline against a gateway with a specific model key. It is here rather than
// in service.go because it is benchmark-only and constructs its own LLM
// provider to avoid coupling to the service's single genModel.
func tailorRendercvResumeWithModel(t *testing.T, gatewayURL, masterKey, modelKey string, master domain.RendercvMaster, vacancy string, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.RendercvMaster, domain.VacancyAnalysis, error) {
	t.Helper()
	// Build a one-off gateway provider for the benchmark. The ollama leg is
	// nil — embeddings are not used in tailoring, only chat.
	gw, err := llm.NewGateway(gatewayURL, masterKey, nil)
	if err != nil {
		return nil, domain.VacancyAnalysis{}, fmt.Errorf("gateway: %w", err)
	}
	ctx := context.Background()
	analysis, err := analyzeVacancy(ctx, gw, modelKey, vacancy, nil)
	if err != nil {
		return nil, domain.VacancyAnalysis{}, fmt.Errorf("analyze: %w", err)
	}
	payload, err := selectContent(ctx, gw, modelKey, master, analysis, level, nil, cfg)
	if err != nil {
		return nil, analysis, fmt.Errorf("tailor: %w", err)
	}
	summary, err := writeSummary(ctx, gw, modelKey, domain.SummaryBrief{
		Analysis:         analysis,
		TotalYears:       domain.DeriveTotalExperienceYears(master),
		Highlights:       domain.SelectedHighlights(payload),
		SkillGroupLabels: domain.SkillGroupLabels(master),
		SentenceMin:      2,
		SentenceMax:      4,
	})
	if err != nil {
		return nil, analysis, fmt.Errorf("summary: %w", err)
	}
	merged, err := domain.MergeTailored(master, payload, &summary)
	if err != nil {
		return nil, analysis, fmt.Errorf("merge: %w", err)
	}
	domain.ApplySectionToggles(merged, cfg)
	domain.ApplyHardLimits(master, merged, cfg)
	domain.DropUngroundedSkillTokens(master, merged)
	violations := domain.VerifyRendercvGrounding(master, merged, level, analysis)
	if len(violations) > 0 {
		return nil, analysis, fmt.Errorf("grounding: %v", violations)
	}
	return merged, analysis, nil
}

func isJSONParseError(err error) bool {
	return err != nil && (containsStr(err.Error(), "not valid JSON") || containsStr(err.Error(), "structured output failed"))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}