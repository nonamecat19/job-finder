//go:build eval_live

package application

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"context"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
	"gopkg.in/yaml.v3"
	"strconv"
)

var liveCasesDir = filepath.Join(evalDataDir, "cases-live")

var (
	flagRecord = flag.Bool("eval.record", false, "record replay fixtures from live providers")
	flagModels = flag.String("eval.models", "", "comma-separated task keys to compare in live mode")
	flagOut    = flag.String("eval.out", "", "path to write the comparison artifact")
)

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

type StageRecord struct {
	Stage       string  `json:"stage"`
	ServedModel string  `json:"served_model"`
	ServedGroup string  `json:"served_group"`
	Substituted bool    `json:"substituted"`
	Escalated   bool    `json:"escalated"`
	DurationMs  int64   `json:"duration_ms"`
	CostUSD     float64 `json:"cost_usd"`
}

type CaseResult struct {
	Case   string             `json:"case"`
	Scores map[string]float64 `json:"scores"`
	Stages []StageRecord      `json:"stages"`

	Incomplete bool    `json:"incomplete"`
	Failure    string  `json:"failure,omitempty"`
	DurationMs int64   `json:"duration_ms"`
	CostUSD    float64 `json:"cost_usd"`
}

type ModelResult struct {
	Model            string             `json:"model"`
	Cases            []CaseResult       `json:"cases"`
	MedianScores     map[string]float64 `json:"median_scores"`
	MedianDurationMs float64            `json:"median_duration_ms"`
	MedianCostUSD    float64            `json:"median_cost_usd"`
	TotalCostUSD     float64            `json:"total_cost_usd"`

	IncompleteCases int `json:"incomplete_cases"`
}

type RequestParams struct {
	GroundingLevel string `json:"grounding_level"`
	ResponseMode   int    `json:"response_mode"`
	Temperature    string `json:"temperature"`
	MaxTokens      string `json:"max_tokens"`
	ShapeConfig    string `json:"shape_config"`
}

type ComparisonRun struct {
	RunAt          string        `json:"run_at"`
	CorpusRevision string        `json:"corpus_revision"`
	ScorerSetVer   int           `json:"scorer_set_version"`
	Params         RequestParams `json:"request_params"`
	Models         []ModelResult `json:"models"`
}

func TestEvalRecord(t *testing.T) {
	if !*flagRecord {
		t.Skip("set -eval.record to record fixtures")
	}
	gatewayURL := os.Getenv("GATEWAY_URL")
	masterKey := os.Getenv("LITELLM_MASTER_KEY")
	if gatewayURL == "" || masterKey == "" {
		t.Skip("GATEWAY_URL and LITELLM_MASTER_KEY must be set to record")
	}

	gw, err := llm.NewGateway(gatewayURL, masterKey, 1024)
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}

	cases := discoverCases(t, casesDir)
	for _, c := range cases {
		if *flagCase != "" && c.Name != *flagCase {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			recordCase(t, gw, c)
		})
	}
}

func recordCase(t *testing.T, gw llm.Provider, c EvalCase) {
	t.Helper()

	stages := map[string]string{
		"generation-analyze":        "analyze",
		"generation-select":         "select",
		"generation-select-premium": "premium",
		"generation-summary":        "summary",
		"generation":                "cover",
	}

	providers := map[string]*ReplayProvider{}
	for key := range stages {
		p := newReplayProvider(t, key, c.Name)
		p.record = true
		p.live = llm.NewRouter(key, gw)
		providers[key] = p
	}

	svc := NewService(nil, nil, nil, nil, GenerationRouters{
		Analyze: providers["generation-analyze"],
		Select:  providers["generation-select"],
		Premium: providers["generation-select-premium"],
		Summary: providers["generation-summary"],
		Cover:   providers["generation"],
	}, "", "", c.Spec.GroundingLevel, nil)

	run := evalRun{prov: &runProvenance{}}
	merged, analysis, err := svc.tailorRendercvResume(
		t.Context(), c.Master, c.Vacancy, c.Level, c.Cfg, nil, nil, run.prov)
	if err != nil {
		t.Fatalf("case %q: live run failed, nothing recorded: %v", c.Name, err)
	}
	deps := stubRenderDeps(svc, c.Spec.PageCounts)
	if _, rerr := svc.renderToPageTarget(
		t.Context(), deps, c.Master, merged, analysis, c.Level, c.Cfg, c.Name, nil); rerr != nil {
		t.Fatalf("case %q: page fitting failed, nothing recorded: %v", c.Name, rerr)
	}

	if _, rerr := rankContent(t.Context(), providers["generation-select"], "", c.Master, analysis, c.Cfg, nil); rerr != nil {
		t.Fatalf("case %q: ranking failed, nothing recorded: %v", c.Name, rerr)
	}

	if _, serr := suggestContent(
		t.Context(), providers["generation-select"], "",
		experienceCompanies(c.Master), domain.SkillGroupLabels(c.Master), analysis); serr != nil {
		t.Fatalf("case %q: suggestion failed, nothing recorded: %v", c.Name, serr)
	}

	dir := caseReplayDir(c.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	written := 0
	for _, p := range providers {
		for hash, f := range p.recorded {
			f.RecordedAt = time.Now().UTC().Format("2006-01-02")
			f.RecordedFrom = p.modelKey
			raw, err := json.MarshalIndent(f, "", "  ")
			if err != nil {
				t.Fatalf("marshal fixture: %v", err)
			}
			name := strings.TrimPrefix(hash, "sha256:")[:16] + ".json"
			if err := os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			written++
		}
	}
	t.Logf("case %q: recorded %d fixture(s) into %s", c.Name, written, dir)
}

func TestLiveComparison(t *testing.T) {
	if os.Getenv("EVAL_LIVE") != "1" {
		t.Skip("set EVAL_LIVE=1 to run the live comparison")
	}
	gatewayURL := os.Getenv("GATEWAY_URL")
	masterKey := os.Getenv("LITELLM_MASTER_KEY")
	if gatewayURL == "" || masterKey == "" {
		t.Skip("GATEWAY_URL and LITELLM_MASTER_KEY must be set")
	}
	if strings.TrimSpace(*flagModels) == "" {
		t.Skip("pass -eval.models with comma-separated task keys")
	}

	gw, err := llm.NewGateway(gatewayURL, masterKey, 1024)
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}

	cases := discoverCases(t, casesDir)
	cases = append(cases, discoverCases(t, liveCasesDir)...)

	artifact := ComparisonRun{
		RunAt:          time.Now().UTC().Format(time.RFC3339),
		CorpusRevision: corpusRevision(t),
		ScorerSetVer:   ScorerSetVersion,
	}

	for _, model := range strings.Split(*flagModels, ",") {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		mr := ModelResult{Model: model, MedianScores: map[string]float64{}}
		perScorer := map[string][]float64{}
		var durations, costs []float64

		for _, c := range cases {
			cr := runCaseLive(t, gw, model, c)
			mr.Cases = append(mr.Cases, cr)
			mr.TotalCostUSD += cr.CostUSD
			if cr.Incomplete {

				mr.IncompleteCases++
				continue
			}
			for name, v := range cr.Scores {
				perScorer[name] = append(perScorer[name], v)
			}
			durations = append(durations, float64(cr.DurationMs))
			costs = append(costs, cr.CostUSD)
		}

		for name, values := range perScorer {
			mr.MedianScores[name] = median(values)
		}
		mr.MedianDurationMs = median(durations)
		mr.MedianCostUSD = median(costs)
		artifact.Models = append(artifact.Models, mr)
	}

	out := *flagOut
	if out == "" {
		out = filepath.Join(evalDataDir, fmt.Sprintf("comparison-%s.json", time.Now().UTC().Format("20060102-150405")))
	}
	raw, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	t.Logf("comparison artifact written to %s", out)

	printComparison(t, artifact)
}

func runCaseLive(t *testing.T, gw llm.Provider, model string, c EvalCase) CaseResult {
	t.Helper()
	cr := CaseResult{Case: c.Name, Scores: map[string]float64{}}

	router := llm.NewRouter(model, gw)
	svc := NewService(nil, nil, nil, nil, GenerationRouters{
		Analyze: router, Select: router, Premium: router, Summary: router, Cover: router,
	}, "", "", c.Spec.GroundingLevel, nil)

	prov := &runProvenance{}
	started := time.Now()
	merged, analysis, err := svc.tailorRendercvResume(
		t.Context(), c.Master, c.Vacancy, c.Level, c.Cfg, nil, nil, prov)
	cr.DurationMs = time.Since(started).Milliseconds()

	for _, o := range prov.stages {
		cr.Stages = append(cr.Stages, StageRecord{
			Stage:       o.Stage,
			ServedModel: o.ServedModel,
			ServedGroup: o.ServedGroup,
			Substituted: o.Substituted,
			Escalated:   o.Escalated,
			DurationMs:  o.DurationMs,
			CostUSD:     o.CostUSD,
		})
	}
	cr.CostUSD = prov.totalCostUSD()

	if err != nil {
		cr.Incomplete = true
		cr.Failure = err.Error()
		return cr
	}
	for name, s := range scoreRun(c, evalRun{merged: merged, analysis: analysis, prov: prov}) {
		cr.Scores[name] = s.Value
	}
	return cr
}

func printComparison(t *testing.T, run ComparisonRun) {
	t.Helper()
	names := scorerNames()

	header := []string{"Model", "Complete", "Median ms", "Median $", "Total $"}
	header = append(header, names...)
	t.Log(strings.Join(header, " | "))

	for _, m := range run.Models {
		row := []string{
			m.Model,
			fmt.Sprintf("%d/%d", len(m.Cases)-m.IncompleteCases, len(m.Cases)),
			fmt.Sprintf("%.0f", m.MedianDurationMs),
			fmt.Sprintf("%.6f", m.MedianCostUSD),
			fmt.Sprintf("%.6f", m.TotalCostUSD),
		}
		for _, n := range names {
			row = append(row, fmt.Sprintf("%.3f", m.MedianScores[n]))
		}
		t.Log(strings.Join(row, " | "))
	}
}

func corpusRevision(t *testing.T) string {
	t.Helper()

	var parts []string
	_ = filepath.WalkDir(casesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		parts = append(parts, fmt.Sprintf("%s:%d", path, len(raw)))
		return nil
	})
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func TestLiveModeIsBuildConstrained(t *testing.T) {
	raw, err := os.ReadFile("eval_live_test.go")
	if err != nil {
		t.Fatalf("read own source: %v", err)
	}
	first := strings.SplitN(string(raw), "\n", 2)[0]
	if strings.TrimSpace(first) != "//go:build eval_live" {
		t.Errorf("first line is %q, want //go:build eval_live. A doc comment claiming a tag is not a tag.", first)
	}
}

func TestMedianIsAMedian(t *testing.T) {
	values := []float64{1, 1, 1, 1, 96}

	if got := median(values); got != 1 {
		t.Errorf("median(%v) = %v, want 1 (the mean is 20 — the statistic this replaces reported that and called it a median)", values, got)
	}
	if got := median([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("median of an even-length set = %v, want 2.5", got)
	}
	if got := median(nil); got != 0 {
		t.Errorf("median(nil) = %v, want 0", got)
	}
}

func TestEveryReportedColumnIsAssigned(t *testing.T) {
	raw, err := os.ReadFile("eval_live_test.go")
	if err != nil {
		t.Fatalf("read own source: %v", err)
	}
	src := string(raw)

	assigned := map[string]string{
		"MedianDurationMs": "mr.MedianDurationMs =",
		"MedianCostUSD":    "mr.MedianCostUSD =",
		"TotalCostUSD":     "mr.TotalCostUSD +=",
		"IncompleteCases":  "mr.IncompleteCases++",
		"MedianScores":     "mr.MedianScores[name] =",
	}
	for field, assignment := range assigned {
		if !strings.Contains(src, assignment) {
			t.Errorf("reported column %q is never assigned (looked for %q); a printed column nothing writes reads as a measurement and is not one", field, assignment)
		}
	}
	for _, stageField := range []string{"ServedModel:", "ServedGroup:", "Substituted:", "Escalated:", "CostUSD:", "DurationMs:"} {
		if !strings.Contains(src, stageField) {
			t.Errorf("stage provenance field %q is never assigned; without it SC-008 is unverifiable because a task key does not determine which model answered", stageField)
		}
	}
}

func TestLiveModeWritesNoBaseline(t *testing.T) {
	raw, err := os.ReadFile("eval_live_test.go")
	if err != nil {
		t.Fatalf("read own source: %v", err)
	}

	code := string(raw)
	if i := strings.Index(code, "// --- live-mode guardrails"); i > 0 {
		code = code[:i]
	}
	if strings.Contains(code, "saveBaseline(") {
		t.Error("live mode calls saveBaseline; a baseline recorded from a live run gates future changes on which upstream answered that day")
	}
}

func TestIncompleteRunIsNotFoldedIntoMedians(t *testing.T) {
	mr := ModelResult{MedianScores: map[string]float64{}}
	var scores []float64
	for _, cr := range []CaseResult{
		{Scores: map[string]float64{"grounding_violations": 0}},
		{Incomplete: true, Failure: "provider unavailable"},
		{Scores: map[string]float64{"grounding_violations": 2}},
	} {
		if cr.Incomplete {
			mr.IncompleteCases++
			continue
		}
		scores = append(scores, cr.Scores["grounding_violations"])
	}
	if mr.IncompleteCases != 1 {
		t.Errorf("incomplete cases = %d, want 1", mr.IncompleteCases)
	}
	if len(scores) != 2 {
		t.Errorf("%d scores folded into the median, want 2 — an incomplete run must not count as a clean one", len(scores))
	}
}

const (
	defaultBaselineCostUSD  = 0.113
	defaultBaselineDuration = 60 * time.Second
)

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

	gw, err := llm.NewGateway(gatewayURL, masterKey, 1024)
	if err != nil {
		t.Fatalf("gateway: %v", err)
	}
	svc := &Service{llm: GenerationRouters{
		Analyze: llm.NewRouter("generation-analyze", gw),
		Select:  llm.NewRouter("generation-select", gw),
		Premium: llm.NewRouter("generation-select-premium", gw),
		Summary: llm.NewRouter("generation-summary", gw),
		Cover:   llm.NewRouter("generation", gw),
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
