package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The routing contract Constitution V depends on lives in a file the Go build
// never touches, so nothing but this test stands between a forgotten fallback
// chain and a generation stage that terminates on a hosted provider.
// specs/035-split-model-generation/contracts/contracts.md §1.
//
// Each invariant is a pure func returning its violations, so the checks
// themselves are tested against inline fixtures below — a guardrail that can
// only ever pass guards nothing.

type gatewayConfig struct {
	ModelList []struct {
		ModelName string         `yaml:"model_name"`
		Params    map[string]any `yaml:"litellm_params"`
		ModelInfo map[string]any `yaml:"model_info"`
	} `yaml:"model_list"`
	LiteLLMSettings struct {
		Fallbacks       []map[string][]string `yaml:"fallbacks"`
		SuccessCallback []string              `yaml:"success_callback"`
		FailureCallback []string              `yaml:"failure_callback"`
		RequestTimeout  int                   `yaml:"request_timeout"`
		NumRetries      int                   `yaml:"num_retries"`
		AllowedFails    int                   `yaml:"allowed_fails"`
		CooldownTime    int                   `yaml:"cooldown_time"`
	} `yaml:"litellm_settings"`
}

// requestedGenerationGroups are the keys the application sends as the `model`
// field (cmd/server/compose.go). Listed explicitly so deleting a chain AND its
// group still fails rather than quietly satisfying the derived check.
var requestedGenerationGroups = []string{
	"generation",
	"generation-analyze",
	"generation-select",
	"generation-select-premium",
	"generation-summary",
	// 034: the two hosted summary options a user can pick. The `standard`
	// option routes to "generation-summary" above and needs no entry of its
	// own; the self-hosted option never reaches the gateway at all.
	"generation-summary-premium",
	"generation-summary-fast",
}

const terminalTier = "local"

func parseGatewayConfig(raw []byte) (*gatewayConfig, error) {
	cfg := &gatewayConfig{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, err
	}
	if len(cfg.ModelList) == 0 {
		return nil, fmt.Errorf("model_list is empty")
	}
	if len(cfg.LiteLLMSettings.Fallbacks) == 0 {
		return nil, fmt.Errorf("litellm_settings.fallbacks is empty")
	}
	return cfg, nil
}

func loadGatewayConfig(t *testing.T) *gatewayConfig {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}
	// .../apps/api/internal/platform/llm -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", ".."))
	path := filepath.Join(root, "gateway", "config.yaml")

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("gateway config not present at %s; skipping routing guardrails", path)
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	cfg, err := parseGatewayConfig(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cfg
}

func (c *gatewayConfig) chains() map[string][]string {
	out := make(map[string][]string, len(c.LiteLLMSettings.Fallbacks))
	for _, entry := range c.LiteLLMSettings.Fallbacks {
		for group, chain := range entry {
			out[group] = chain
		}
	}
	return out
}

func (c *gatewayConfig) deployments() map[string]map[string]any {
	out := make(map[string]map[string]any, len(c.ModelList))
	for _, d := range c.ModelList {
		out[d.ModelName] = d.Params
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Invariant 1: every group the application requests has a chain.
func checkRequestedGroupsHaveChains(c *gatewayConfig) []string {
	var violations []string
	chains := c.chains()
	deployments := c.deployments()

	for _, group := range requestedGenerationGroups {
		if _, ok := deployments[group]; !ok {
			violations = append(violations, fmt.Sprintf(
				"model group %q is requested by the application but has no model_list entry", group))
		}
		if _, ok := chains[group]; !ok {
			violations = append(violations, fmt.Sprintf(
				"model group %q has no litellm_settings.fallbacks chain; without one it terminates on its own hosted provider instead of %q (FR-011)",
				group, terminalTier))
		}
	}

	// Derived check: a generation-* group that is neither a chain key nor a
	// member of some other group's chain is unreachable by fallback, which
	// means it is a requested group whose chain was forgotten.
	inSomeChain := map[string]bool{}
	for _, chain := range chains {
		for _, tier := range chain {
			inSomeChain[tier] = true
		}
	}
	for _, group := range sortedKeys(deployments) {
		if !strings.HasPrefix(group, "generation") || inSomeChain[group] {
			continue
		}
		if _, ok := chains[group]; !ok {
			violations = append(violations, fmt.Sprintf(
				"model group %q appears in no fallback chain and declares none of its own: either it is a requested key missing its chain, or dead configuration (FR-011)",
				group))
		}
	}
	return violations
}

// Invariant 2: every chain ends at the shared local model.
func checkChainsTerminateAtLocal(c *gatewayConfig) []string {
	var violations []string
	if _, ok := c.deployments()[terminalTier]; !ok {
		violations = append(violations, fmt.Sprintf(
			"model_list has no %q deployment; no chain can terminate locally (FR-011)", terminalTier))
	}

	chains := c.chains()
	for _, group := range sortedKeys(chains) {
		chain := chains[group]
		if len(chain) == 0 {
			violations = append(violations, fmt.Sprintf(
				"fallback chain for %q is empty; the group has no fallback at all (FR-011)", group))
			continue
		}
		if last := chain[len(chain)-1]; last != terminalTier {
			violations = append(violations, fmt.Sprintf(
				"fallback chain for %q ends at %q, not %q: chain %v terminates on a hosted provider, so a run cannot complete when every hosted option is down (FR-011, Constitution V)",
				group, last, terminalTier, chain))
		}
		for i, tier := range chain[:len(chain)-1] {
			if tier == terminalTier {
				violations = append(violations, fmt.Sprintf(
					"fallback chain for %q lists %q at position %d, before the end: every tier after it is unreachable (chain %v)",
					group, terminalTier, i, chain))
			}
		}
	}
	return violations
}

// Invariant 3: no chain names a tier that does not exist.
func checkChainTiersAreDeclared(c *gatewayConfig) []string {
	var violations []string
	deployments := c.deployments()
	chains := c.chains()
	for _, group := range sortedKeys(chains) {
		for _, tier := range chains[group] {
			if _, ok := deployments[tier]; !ok {
				violations = append(violations, fmt.Sprintf(
					"fallback chain for %q names tier %q, which has no model_list entry; the proxy cannot route to it and the chain effectively ends early",
					group, tier))
			}
		}
	}
	return violations
}

// Invariant 4: a thinking model with no reasoning bound spends its whole
// output budget deliberating and returns empty content. That is what broke
// every resume run before 2026-08-07 (FR-014, research.md R2). Scoped to the
// openrouter/* deployments under the generation-* stage keys: the free-tier
// cerebras/cohere/groq ones are shared with other tasks and predate the rule,
// and the bare `generation` group (cover letter) is outside 035's scope.
func checkGenerationReasoningBounds(c *gatewayConfig) []string {
	var violations []string
	for _, d := range c.ModelList {
		if !strings.HasPrefix(d.ModelName, "generation-") {
			continue
		}
		model, _ := d.Params["model"].(string)
		if !strings.HasPrefix(model, "openrouter/") {
			continue
		}
		_, hasEffort := d.Params["reasoning_effort"]
		_, hasBlock := d.Params["reasoning"]
		if !hasEffort && !hasBlock {
			violations = append(violations, fmt.Sprintf(
				"deployment %q (%s) declares neither reasoning_effort nor a reasoning block; an unbounded thinking model returns empty content because reasoning tokens count against max_completion_tokens (FR-014)",
				d.ModelName, model))
		}
	}
	return violations
}

// Invariant 5: credentials are environment references, never literals (030-C4).
func checkNoLiteralAPIKeys(c *gatewayConfig) []string {
	var violations []string
	for _, d := range c.ModelList {
		key, ok := d.Params["api_key"].(string)
		if !ok {
			violations = append(violations, fmt.Sprintf("deployment %q declares no api_key", d.ModelName))
			continue
		}
		if !strings.HasPrefix(key, "os.environ/") {
			violations = append(violations, fmt.Sprintf(
				"deployment %q has a literal api_key; every key must be an os.environ/… reference (030-C4)",
				d.ModelName))
		}
	}
	return violations
}

var gatewayInvariants = []struct {
	name  string
	check func(*gatewayConfig) []string
}{
	{"every requested generation group has a fallback chain", checkRequestedGroupsHaveChains},
	{"every chain terminates at local", checkChainsTerminateAtLocal},
	{"every chain tier is declared in model_list", checkChainTiersAreDeclared},
	{"every generation-* openrouter deployment bounds reasoning", checkGenerationReasoningBounds},
	{"no literal api keys", checkNoLiteralAPIKeys},
	{"observability callbacks are declared globally", checkObservabilityCallbacks},
	{"036 did not disturb the failover timing arithmetic", checkTimingArithmeticUnchanged},
	{"the local terminal tier declares a zero cost", checkLocalTierIsPricedFree},
	{"every tier of a tool-using chain declares tool capability", checkToolChainsDeclareCapability},
}

// toolUsingTaskKeys are the task keys an application tool loop runs on (037
// FR-018). Listed explicitly, in the style of requestedGenerationGroups: a
// derived list would go quiet the moment a consumer was removed, which is
// exactly when somebody is most likely to have forgotten the annotation.
var toolUsingTaskKeys = []string{"default"}

// Invariant 6: both callbacks declared, and declared globally (036-C1-1/C1-2).
//
// Success-only would hide the failures FR-002 requires, and a call that
// exhausts every tier is exactly the one worth a record — a failure that
// produces nothing is indistinguishable from a call that never happened.
func checkObservabilityCallbacks(c *gatewayConfig) []string {
	var violations []string
	has := func(list []string) bool {
		for _, v := range list {
			if v == "langfuse" {
				return true
			}
		}
		return false
	}
	if !has(c.LiteLLMSettings.SuccessCallback) {
		violations = append(violations, "litellm_settings.success_callback does not list langfuse (036-C1-1)")
	}
	if !has(c.LiteLLMSettings.FailureCallback) {
		violations = append(violations, "litellm_settings.failure_callback does not list langfuse; failures must be recorded too (036-C1-1)")
	}
	// Per-deployment callbacks would make coverage depend on which tier
	// answered, which is the opposite of what an observability layer is for.
	for _, d := range c.ModelList {
		for _, k := range []string{"success_callback", "failure_callback", "callbacks"} {
			if _, present := d.Params[k]; present {
				violations = append(violations, fmt.Sprintf(
					"deployment %q declares %s; callbacks are global only (036-C1-2)", d.ModelName, k))
			}
		}
	}
	return violations
}

// Invariant 7: the worst-case failover arithmetic is unchanged (036-C1-4).
//
// tiers x (1 + num_retries) x request_timeout must stay under the Go adapter's
// 15-minute safety net so the proxy is always what times out first. Adding
// observability must not perturb any term. These literals are pinned
// deliberately: a silent change here is how the 830-second hang came back.
func checkTimingArithmeticUnchanged(c *gatewayConfig) []string {
	var violations []string
	for _, want := range []struct {
		name string
		got  int
		want int
	}{
		{"request_timeout", c.LiteLLMSettings.RequestTimeout, 60},
		{"num_retries", c.LiteLLMSettings.NumRetries, 1},
		{"allowed_fails", c.LiteLLMSettings.AllowedFails, 3},
		{"cooldown_time", c.LiteLLMSettings.CooldownTime, 60},
	} {
		if want.got != want.want {
			violations = append(violations, fmt.Sprintf(
				"litellm_settings.%s is %d, want %d; changing it requires redoing the worst-case timing arithmetic (036-C1-4)",
				want.name, want.got, want.want))
		}
	}
	return violations
}

// Invariant 8: local declares an explicit zero cost (036-FR-014).
//
// Without it the proxy emits no cost for a deployment absent from its cost
// map, making a free call and an unpriced one indistinguishable in the record.
func checkLocalTierIsPricedFree(c *gatewayConfig) []string {
	for _, d := range c.ModelList {
		if d.ModelName != terminalTier {
			continue
		}
		for _, k := range []string{"input_cost_per_token", "output_cost_per_token"} {
			v, ok := d.ModelInfo[k]
			if !ok {
				return []string{fmt.Sprintf("deployment %q declares no model_info.%s; a free call and an unpriced one would be indistinguishable (036-FR-014)", terminalTier, k)}
			}
			if n, isInt := v.(int); !isInt || n != 0 {
				return []string{fmt.Sprintf("deployment %q has model_info.%s = %v, want 0", terminalTier, k, v)}
			}
		}
		return nil
	}
	return []string{fmt.Sprintf("no %q deployment found", terminalTier)}
}

// Invariant 9: every tier of a tool-using chain says whether it can call tools
// (037 FR-018, C6-1/C6-2/C6-5).
//
// What this asserts is that somebody *considered* the question when adding a
// tier — not that the proxy will act on the answer. It will not: LiteLLM reads
// model_info for its model-info endpoint and cost bookkeeping, and
// `drop_params: true` silently drops a `tools` array an upstream will not
// accept, so the request succeeds without tools and the fallback chain never
// engages. That is the same capability trap this repository already documents
// for response_format.
//
// The runtime backstop is elsewhere: the loop's first round is sent with
// tool_choice "required", and prose in reply to it returns not_tool_capable
// rather than an answer.
func checkToolChainsDeclareCapability(c *gatewayConfig) []string {
	declared := map[string]bool{}
	for _, d := range c.ModelList {
		if v, ok := d.ModelInfo["supports_function_calling"]; ok {
			if b, isBool := v.(bool); isBool && b {
				declared[d.ModelName] = true
			}
		}
	}

	var violations []string
	for _, key := range toolUsingTaskKeys {
		chain := append([]string{key}, chainFor(c, key)...)
		for _, tier := range chain {
			if !declared[tier] {
				violations = append(violations, fmt.Sprintf(
					"tier %q of tool-using chain %q declares no model_info.supports_function_calling: true; "+
						"a tier added to a tool chain without a capability decision fails the loop's required first round at runtime (037-FR-018)",
					tier, key))
			}
		}
		if len(chain) > 0 && chain[len(chain)-1] != terminalTier {
			violations = append(violations, fmt.Sprintf(
				"tool-using chain %q does not terminate at %q; Principle V holds for tool chains too (037-C6-3)", key, terminalTier))
		}
	}
	return violations
}

// chainFor returns the declared fallback chain for a task key, or nil.
func chainFor(c *gatewayConfig, key string) []string {
	for _, entry := range c.LiteLLMSettings.Fallbacks {
		if chain, ok := entry[key]; ok {
			return chain
		}
	}
	return nil
}

func TestGatewayConfigHonoursRoutingContract(t *testing.T) {
	cfg := loadGatewayConfig(t)
	for _, inv := range gatewayInvariants {
		t.Run(inv.name, func(t *testing.T) {
			for _, v := range inv.check(cfg) {
				t.Error(v)
			}
		})
	}
}

// --- the guardrails' own guardrails -----------------------------------------

const validFixture = `
model_list:
  - model_name: generation
    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-cerebras
    litellm_params: {model: cerebras/gpt-oss-120b, api_key: os.environ/CEREBRAS_API_KEY}
  - model_name: generation-analyze
    litellm_params: {model: openrouter/google/gemini-2.5-flash-lite, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-select
    litellm_params: {model: openrouter/google/gemini-2.5-flash-lite, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-select-premium
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-premium
    litellm_params: {model: openrouter/anthropic/claude-opus-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-fast
    litellm_params: {model: cerebras/gpt-oss-120b, api_key: os.environ/CEREBRAS_API_KEY}
  - model_name: default
    litellm_params: {model: cerebras/gpt-oss-120b, api_key: os.environ/CEREBRAS_API_KEY}
    model_info: {supports_function_calling: true}
  - model_name: default-groq
    litellm_params: {model: groq/llama-3.3-70b-versatile, api_key: os.environ/GROQ_API_KEY}
    model_info: {supports_function_calling: true}
  - model_name: local
    litellm_params: {model: ollama_chat/gpt-oss:120b-cloud, api_key: os.environ/OLLAMA_KEY}
    model_info: {input_cost_per_token: 0, output_cost_per_token: 0, supports_function_calling: true}
litellm_settings:
  success_callback: ["langfuse"]
  failure_callback: ["langfuse"]
  request_timeout: 60
  num_retries: 1
  allowed_fails: 3
  cooldown_time: 60
  fallbacks:
    - generation: [generation-cerebras, local]
    - generation-analyze: [local]
    - generation-select: [local]
    - generation-select-premium: [local]
    - generation-summary: [local]
    - generation-summary-premium: [local]
    - generation-summary-fast: [local]
    - default: [default-groq, local]
`

func fixture(t *testing.T, yamlText string) *gatewayConfig {
	t.Helper()
	cfg, err := parseGatewayConfig([]byte(yamlText))
	if err != nil {
		t.Fatalf("fixture parse: %v", err)
	}
	return cfg
}

func TestGatewayInvariantsAcceptValidConfig(t *testing.T) {
	cfg := fixture(t, validFixture)
	for _, inv := range gatewayInvariants {
		if v := inv.check(cfg); len(v) > 0 {
			t.Errorf("%s: valid fixture rejected: %v", inv.name, v)
		}
	}
}

func TestGatewayInvariantsRejectBrokenConfig(t *testing.T) {
	tests := []struct {
		name    string
		check   func(*gatewayConfig) []string
		mutate  func(string) string
		wantSub string
	}{
		{
			name:    "chain dropped for the escalation key",
			check:   checkRequestedGroupsHaveChains,
			mutate:  func(s string) string { return strings.Replace(s, "    - generation-select-premium: [local]\n", "", 1) },
			wantSub: `"generation-select-premium" has no litellm_settings.fallbacks chain`,
		},
		{
			name:  "chain stops at a hosted provider",
			check: checkChainsTerminateAtLocal,
			mutate: func(s string) string {
				return strings.Replace(s, "generation-summary: [local]", "generation-summary: [generation-cerebras]", 1)
			},
			// This is the defect the whole file exists to catch.
			wantSub: `ends at "generation-cerebras", not "local"`,
		},
		{
			name:  "chain names an undeclared tier",
			check: checkChainTiersAreDeclared,
			mutate: func(s string) string {
				return strings.Replace(s, "generation-analyze: [local]", "generation-analyze: [generation-analyze-typo, local]", 1)
			},
			wantSub: `names tier "generation-analyze-typo"`,
		},
		{
			name:  "stage deployment left unbounded",
			check: checkGenerationReasoningBounds,
			mutate: func(s string) string {
				return strings.Replace(s, "model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low", "model: openrouter/anthropic/claude-sonnet-5", 2)
			},
			wantSub: "declares neither reasoning_effort nor a reasoning block",
		},
		{
			name:    "literal api key",
			check:   checkNoLiteralAPIKeys,
			mutate:  func(s string) string { return strings.Replace(s, "os.environ/OLLAMA_KEY", "sk-real-secret", 1) },
			wantSub: "has a literal api_key",
		},
		{
			name:    "success callback dropped",
			check:   checkObservabilityCallbacks,
			mutate:  func(s string) string { return strings.Replace(s, "  success_callback: [\"langfuse\"]\n", "", 1) },
			wantSub: "success_callback does not list langfuse",
		},
		{
			// Recording only successes hides the calls most worth a record.
			name:    "failure callback dropped",
			check:   checkObservabilityCallbacks,
			mutate:  func(s string) string { return strings.Replace(s, "  failure_callback: [\"langfuse\"]\n", "", 1) },
			wantSub: "failure_callback does not list langfuse",
		},
		{
			name:  "callback attached to a single deployment",
			check: checkObservabilityCallbacks,
			mutate: func(s string) string {
				return strings.Replace(s,
					"litellm_params: {model: ollama_chat/gpt-oss:120b-cloud, api_key: os.environ/OLLAMA_KEY}",
					"litellm_params: {model: ollama_chat/gpt-oss:120b-cloud, api_key: os.environ/OLLAMA_KEY, success_callback: [langfuse]}", 1)
			},
			wantSub: "callbacks are global only",
		},
		{
			// The failure mode this pins: raising a timeout silently pushes the
			// worst case past the Go adapter's safety net, which is how a call
			// once hung for 830 seconds.
			name:    "request_timeout quietly raised",
			check:   checkTimingArithmeticUnchanged,
			mutate:  func(s string) string { return strings.Replace(s, "request_timeout: 60", "request_timeout: 300", 1) },
			wantSub: "request_timeout is 300, want 60",
		},
		{
			name:  "local tier loses its zero cost",
			check: checkLocalTierIsPricedFree,
			mutate: func(s string) string {
				return strings.Replace(s, "    model_info: {input_cost_per_token: 0, output_cost_per_token: 0, supports_function_calling: true}\n", "", 1)
			},
			wantSub: "declares no model_info.input_cost_per_token",
		},
		{
			// The case C6-5 names: a tier added to a tool chain without a
			// capability decision. It answers the loop's required first round
			// with prose, which reads as a model problem rather than a config
			// one unless something says otherwise here.
			name:  "a tool-chain tier is added without a capability declaration",
			check: checkToolChainsDeclareCapability,
			mutate: func(s string) string {
				return strings.Replace(s,
					"    - default: [default-groq, local]",
					"    - default: [default-groq, default-mystery, local]", 1) + "\n"
			},
			wantSub: "declares no model_info.supports_function_calling",
		},
		{
			name:  "the shared local tier loses its capability declaration",
			check: checkToolChainsDeclareCapability,
			mutate: func(s string) string {
				return strings.Replace(s,
					"model_info: {input_cost_per_token: 0, output_cost_per_token: 0, supports_function_calling: true}",
					"model_info: {input_cost_per_token: 0, output_cost_per_token: 0}", 1)
			},
			wantSub: "tier \"local\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutated := tc.mutate(validFixture)
			if mutated == validFixture {
				t.Fatal("mutation did not change the fixture; the test is vacuous")
			}
			violations := tc.check(fixture(t, mutated))
			if len(violations) == 0 {
				t.Fatalf("check accepted a broken config; expected a violation containing %q", tc.wantSub)
			}
			if !strings.Contains(strings.Join(violations, "\n"), tc.wantSub) {
				t.Errorf("violation message %q does not name the cause %q", violations, tc.wantSub)
			}
		})
	}
}
