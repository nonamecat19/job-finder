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
	} `yaml:"model_list"`
	LiteLLMSettings struct {
		Fallbacks []map[string][]string `yaml:"fallbacks"`
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
  - model_name: local
    litellm_params: {model: ollama_chat/gpt-oss:120b-cloud, api_key: os.environ/OLLAMA_KEY}
litellm_settings:
  fallbacks:
    - generation: [generation-cerebras, local]
    - generation-analyze: [local]
    - generation-select: [local]
    - generation-select-premium: [local]
    - generation-summary: [local]
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
			name:   "chain stops at a hosted provider",
			check:  checkChainsTerminateAtLocal,
			mutate: func(s string) string { return strings.Replace(s, "generation-summary: [local]", "generation-summary: [generation-cerebras]", 1) },
			// This is the defect the whole file exists to catch.
			wantSub: `ends at "generation-cerebras", not "local"`,
		},
		{
			name:    "chain names an undeclared tier",
			check:   checkChainTiersAreDeclared,
			mutate:  func(s string) string { return strings.Replace(s, "generation-analyze: [local]", "generation-analyze: [generation-analyze-typo, local]", 1) },
			wantSub: `names tier "generation-analyze-typo"`,
		},
		{
			name:    "stage deployment left unbounded",
			check:   checkGenerationReasoningBounds,
			mutate:  func(s string) string { return strings.Replace(s, "model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low", "model: openrouter/anthropic/claude-sonnet-5", 2) },
			wantSub: "declares neither reasoning_effort nor a reasoning block",
		},
		{
			name:    "literal api key",
			check:   checkNoLiteralAPIKeys,
			mutate:  func(s string) string { return strings.Replace(s, "os.environ/OLLAMA_KEY", "sk-real-secret", 1) },
			wantSub: "has a literal api_key",
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
