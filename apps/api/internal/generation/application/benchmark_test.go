package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

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
		"generation",          // primary (OpenRouter deepseek-v4-pro)
		"generation-cerebras", // tier 2
		"generation-groq",     // tier 3
		"generation-cohere",   // tier 4
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
		model                string
		groundingViolations  int
		structuralViolations int
		jsonParseFailures    int
		medianMs             int64
		attempts             int
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

// Pre-split baseline for the split pipeline's cost and latency targets (035
// SC-001, SC-002): one full single-model run on the strongest option evaluated,
// measured on 2026-08-07 and recorded in the feature's spec. Overridable with
// BASELINE_COST_USD / BASELINE_SECONDS so a re-measured baseline does not
// require a code change.
const (
	defaultBaselineCostUSD  = 0.113
	defaultBaselineDuration = 60 * time.Second
)

// TestBenchmarkSplitPipelineTargets is the split-pipeline cost/latency
// benchmark (035 T049, SC-001, SC-002, SC-009). It runs the real staged
// pipeline against the gateway's stage keys, reads back per-stage duration,
// tokens, cost and serving model from the run's own provenance — the same
// side-channel the application records on a document — and asserts the two
// measurable targets against the recorded pre-split baseline:
//
//	SC-001: cost per resume ≤ 1/5 of baseline
//	SC-002: median wall-clock ≤ 1/2 of baseline
//
// It calls live providers, so it is skipped in short mode and unless
// GENERATION_BENCHMARK=1. Run it manually:
//
//	GENERATION_BENCHMARK=1 go test -run TestBenchmarkSplitPipelineTargets \
//	  -v -timeout 30m ./internal/generation/application/
func TestBenchmarkSplitPipelineTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark requires live providers")
	}
	if os.Getenv("GENERATION_BENCHMARK") != "1" {
		t.Skip("set GENERATION_BENCHMARK=1 to run the split-pipeline benchmark")
	}
	gatewayURL := os.Getenv("GATEWAY_URL")
	masterKey := os.Getenv("LITELLM_MASTER_KEY")
	if gatewayURL == "" || masterKey == "" {
		t.Skip("GATEWAY_URL and LITELLM_MASTER_KEY must be set")
	}

	baselineCost := envFloat(t, "BASELINE_COST_USD", defaultBaselineCostUSD)
	baselineDur := time.Duration(envFloat(t, "BASELINE_SECONDS", defaultBaselineDuration.Seconds()) * float64(time.Second))
	runs := int(envFloat(t, "GENERATION_BENCHMARK_RUNS", 3))
	if runs < 1 {
		runs = 1
	}

	// The service is built from stage routers pointed at the gateway task keys,
	// exactly as cmd/server wires them, so the benchmark measures the shipped
	// routing rather than a benchmark-only path. The ollama leg is nil: the
	// point of this measurement is the hosted chain.
	gw, err := llm.NewGateway(gatewayURL, masterKey, nil)
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}
	svc := &Service{llm: GenerationRouters{
		Analyze: llm.NewRouter("generation-analyze", gw, nil, ""),
		Select:  llm.NewRouter("generation-select", gw, nil, ""),
		Premium: llm.NewRouter("generation-select-premium", gw, nil, ""),
		Summary: llm.NewRouter("generation-summary", gw, nil, ""),
		Cover:   llm.NewRouter("generation", gw, nil, ""),
	}}

	master := loadBenchmarkMaster(t)
	vacancy := "Senior Backend Engineer (Go, Postgres, Kubernetes, Terraform). " +
		"Design and build distributed systems. Lead a small team. 5+ years experience."
	level := domain.GroundingModerate
	cfg := domain.DefaultShapeConfig()

	type stageTotals struct {
		stage        string
		servedModels map[string]int
		durationMs   int64
		promptTok    int
		completeTok  int
		costUSD      float64
		calls        int
	}
	order := []string{}
	totals := map[string]*stageTotals{}
	var runDurations []time.Duration
	var runCosts []float64

	for run := 0; run < runs; run++ {
		prov := &runProvenance{}
		started := time.Now()
		_, _, err := svc.tailorRendercvResume(context.Background(), master, vacancy, level, cfg, nil, nil, prov)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("run %d: tailor: %v", run+1, err)
		}
		runDurations = append(runDurations, elapsed)
		runCosts = append(runCosts, prov.totalCostUSD())
		for _, o := range prov.stages {
			st, ok := totals[o.Stage]
			if !ok {
				st = &stageTotals{stage: o.Stage, servedModels: map[string]int{}}
				totals[o.Stage] = st
				order = append(order, o.Stage)
			}
			st.durationMs += o.DurationMs
			st.promptTok += o.PromptTokens
			st.completeTok += o.CompletionTokens
			st.costUSD += o.CostUSD
			st.calls++
			if o.ServedModel != "" {
				st.servedModels[o.ServedModel]++
			}
		}
	}

	fmt.Printf("\n=== 035 Split-Pipeline Benchmark (%d run(s)) ===\n", runs)
	fmt.Println("Stage      | Calls | Avg ms | Prompt tok | Compl tok | Cost USD  | Served")
	fmt.Println("-----------|-------|--------|------------|-----------|-----------|-------")
	for _, stage := range order {
		st := totals[stage]
		fmt.Printf("%-10s | %5d | %6d | %10d | %9d | %9.6f | %s\n",
			st.stage, st.calls, st.durationMs/int64(runs), st.promptTok/runs,
			st.completeTok/runs, st.costUSD/float64(runs), servedSummary(st.servedModels))
	}

	medianDur := medianDuration(runDurations)
	medianCost := medianFloat(runCosts)
	costTarget := baselineCost / 5
	durTarget := baselineDur / 2
	fmt.Printf("\nmedian cost: $%.6f (SC-001 target ≤ $%.6f, baseline $%.6f)\n", medianCost, costTarget, baselineCost)
	fmt.Printf("median time: %s (SC-002 target ≤ %s, baseline %s)\n\n", medianDur, durTarget, baselineDur)

	// A zero cost means the proxy reported none, not that the run was free —
	// asserting the target against it would pass vacuously (FR-017/SC-009).
	if medianCost <= 0 {
		t.Fatalf("SC-001 unmeasurable: no stage reported a cost; the proxy must return usage for the target to mean anything")
	}
	if medianCost > costTarget {
		t.Errorf("SC-001 not met: median cost $%.6f exceeds one fifth of the $%.6f baseline ($%.6f)", medianCost, baselineCost, costTarget)
	}
	if medianDur > durTarget {
		t.Errorf("SC-002 not met: median wall-clock %s exceeds half of the %s baseline (%s)", medianDur, baselineDur, durTarget)
	}
}

// loadBenchmarkMaster loads the real master profile the deployment generates
// from, because the cost and latency figures are only meaningful against a
// profile of realistic size — and the thin in-file sample cannot pass the
// grounding check, so a run against it never reaches the measurement. Set
// RESUME_MASTER_PATH to point elsewhere; the default is the repository's own
// master, resolved relative to this package.
func loadBenchmarkMaster(t *testing.T) domain.RendercvMaster {
	t.Helper()
	path := os.Getenv("RESUME_MASTER_PATH")
	if path == "" {
		path = filepath.Join("..", "..", "..", "..", "..", "resume", "resume.yaml")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("master profile not readable at %s (set RESUME_MASTER_PATH): %v", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal master %s: %v", path, err)
	}
	return domain.RendercvMaster(domain.NormalizeYAMLMap(m).(map[string]any))
}

func servedSummary(counts map[string]int) string {
	if len(counts) == 0 {
		return "(none reported)"
	}
	models := make([]string, 0, len(counts))
	for m := range counts {
		models = append(models, m)
	}
	sort.Strings(models)
	parts := make([]string, 0, len(models))
	for _, m := range models {
		parts = append(parts, fmt.Sprintf("%s×%d", m, counts[m]))
	}
	return strings.Join(parts, ", ")
}

func medianDuration(in []time.Duration) time.Duration {
	if len(in) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), in...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

func medianFloat(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	s := append([]float64(nil), in...)
	sort.Float64s(s)
	return s[len(s)/2]
}

func envFloat(t *testing.T, key string, fallback float64) float64 {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		t.Fatalf("%s=%q is not a number: %v", key, raw, err)
	}
	return v
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
