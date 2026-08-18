package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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

var requestedScenarioGroups = []string{
	"match",
	"ghost",
	"rephrase",
	"recruiter",
	"salary",
	"outreach",
	"generation",
	"generation-analyze",
	"generation-select",
	"generation-select-premium",
	"generation-summary",
	"generation-summary-premium",
	"generation-summary-fast",
	"embed",
}

var toolCapableTaskKeys = []string{"salary"}

var singleProviderScenarios = map[string]bool{
	"embed":              true,
	"generation-analyze": true,
	"generation-select":  true,
}

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

func appConfigDefaultsSource() ([]byte, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("cannot resolve test file path")
	}

	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "defaults.go")
	return os.ReadFile(path)
}

var embedDimsPattern = regexp.MustCompile(`"EMBED_DIMS":\s*(\d+)`)

func appEmbedDims() (int, error) {
	src, err := appConfigDefaultsSource()
	if err != nil {
		return 0, fmt.Errorf("read internal/config/defaults.go: %w", err)
	}
	m := embedDimsPattern.FindSubmatch(src)
	if m == nil {
		return 0, fmt.Errorf("internal/config/defaults.go declares no EMBED_DIMS default")
	}
	return strconv.Atoi(string(m[1]))
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

func providerOf(params map[string]any) (string, bool) {
	model, _ := params["model"].(string)
	provider, _, found := strings.Cut(model, "/")
	if !found || provider == "" {
		return "", false
	}
	return provider, true
}

func checkRequestedGroupsHaveChains(c *gatewayConfig) []string {
	var violations []string
	chains := c.chains()
	deployments := c.deployments()

	declared := map[string]bool{}
	for _, group := range requestedScenarioGroups {
		declared[group] = true
		if _, ok := deployments[group]; !ok {
			violations = append(violations, fmt.Sprintf(
				"model group %q is requested by the application but has no model_list entry", group))
		}
		if _, ok := chains[group]; !ok && !singleProviderScenarios[group] {
			violations = append(violations, fmt.Sprintf(
				"model group %q has no litellm_settings.fallbacks chain; without one it terminates on its own single provider instead of resolving to a multi-provider chain (contracts/gateway-config.md C2-1)",
				group))
		}
	}

	inSomeChain := map[string]bool{}
	for _, chain := range chains {
		for _, tier := range chain {
			inSomeChain[tier] = true
		}
	}
	for _, group := range sortedKeys(chains) {
		if declared[group] || inSomeChain[group] {
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"model group %q declares a fallback chain but is not in requestedScenarioGroups and is not a fallback tier of any other chain: either it is a requested key missing from the list, or dead configuration (contracts/gateway-config.md C1-4)",
			group))
	}
	return violations
}

func checkChainArityAndProviders(c *gatewayConfig) []string {
	var violations []string
	chains := c.chains()

	for _, group := range sortedKeys(chains) {
		full := append([]string{group}, chains[group]...)
		if len(full) < 2 {
			violations = append(violations, fmt.Sprintf(
				"chain for %q has %d tier(s) total (%v); a declared fallback chain must resolve to at least two tiers (contracts/gateway-config.md C2-1)",
				group, len(full), full))
		}
	}
	return violations
}

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

func checkNoDefaultOrLocalNaming(c *gatewayConfig) []string {
	var violations []string

	for _, group := range sortedKeys(c.chains()) {
		if group == "default" {
			violations = append(violations, fmt.Sprintf(
				"model group %q is named \"default\"; the default/local escape hatch is removed by this feature (contracts/gateway-config.md C1-2)", group))
		}
	}

	for _, d := range c.ModelList {
		if d.ModelName == "default" {
			violations = append(violations, `model_list declares a "default" deployment; the default/local escape hatch is removed by this feature (contracts/gateway-config.md C1-2)`)
		}
		if strings.Contains(strings.ToLower(d.ModelName), "local") {
			violations = append(violations, fmt.Sprintf(
				"deployment %q contains \"local\"; self-hosted runtimes are removed by this feature (contracts/gateway-config.md C2-1)", d.ModelName))
		}
		if provider, ok := providerOf(d.Params); ok && strings.Contains(strings.ToLower(provider), "ollama") {
			violations = append(violations, fmt.Sprintf(
				"deployment %q routes through provider %q, a self-hosted runtime; every tier must be hosted (contracts/gateway-config.md C2-1)", d.ModelName, provider))
		}
	}
	return violations
}

func checkOpenRouterReasoningBounds(c *gatewayConfig) []string {
	var violations []string
	for _, d := range c.ModelList {
		provider, ok := providerOf(d.Params)
		if !ok || provider != "openrouter" {
			continue
		}
		model, _ := d.Params["model"].(string)
		_, hasEffort := d.Params["reasoning_effort"]
		_, hasBlock := d.Params["reasoning"]
		if !hasEffort && !hasBlock {
			violations = append(violations, fmt.Sprintf(
				"deployment %q (%s) declares neither reasoning_effort nor a reasoning block; an unbounded thinking model returns empty content because reasoning tokens count against max_completion_tokens (FR-014, contracts/gateway-config.md C6)",
				d.ModelName, model))
		}
	}
	return violations
}

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

func embedTiers(c *gatewayConfig) []string {
	chain, ok := c.chains()["embed"]
	if !ok {

		return []string{"embed"}
	}
	return append([]string{"embed"}, chain...)
}

func checkEmbedChainDeclarations(c *gatewayConfig) []string {
	tiers := embedTiers(c)
	if len(tiers) == 0 {
		return []string{`no "embed" fallback chain declared; the embedding scenario has no chain (contracts/gateway-config.md C2-4)`}
	}

	var violations []string
	deployments := c.deployments()
	var widths []int
	for _, tier := range tiers {
		params, ok := deployments[tier]
		if !ok {
			continue
		}
		if dim, has := params["output_dimension"]; !has {
			violations = append(violations, fmt.Sprintf(
				"embed tier %q declares no output_dimension (contracts/gateway-config.md C3-4)", tier))
		} else if n, isInt := dim.(int); !isInt {
			violations = append(violations, fmt.Sprintf(
				"embed tier %q has output_dimension %v, which is not an integer (contracts/gateway-config.md C3-4)", tier, dim))
		} else {
			widths = append(widths, n)
		}

		if it, has := params["input_type"]; !has || it != "search_document" {
			violations = append(violations, fmt.Sprintf(
				"embed tier %q does not declare input_type: search_document (got %v) (contracts/gateway-config.md C3-4, research.md R2)", tier, it))
		}
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[0] {
			violations = append(violations, fmt.Sprintf(
				"embed chain output_dimension values disagree across tiers: %v (contracts/gateway-config.md C3-4)", widths))
			break
		}
	}
	return violations
}

func checkEmbedWidthMatchesAppDefault(c *gatewayConfig, wantDims int) []string {
	deployments := c.deployments()
	for _, tier := range embedTiers(c) {
		params, ok := deployments[tier]
		if !ok {
			continue
		}
		dim, isInt := params["output_dimension"].(int)
		if !isInt {
			continue
		}
		if dim != wantDims {
			return []string{fmt.Sprintf(
				"embed tier %q declares output_dimension %d, want %d to match the application's EMBED_DIMS default (contracts/gateway-config.md C5-1)",
				tier, dim, wantDims)}
		}
	}
	return nil
}

var gatewayInvariants = []struct {
	name  string
	check func(*gatewayConfig) []string
}{
	{"every requested scenario has a fallback chain", checkRequestedGroupsHaveChains},
	{"every declared chain has >=2 tiers", checkChainArityAndProviders},
	{"no group named default, no tier named local", checkNoDefaultOrLocalNaming},
	{"every chain tier is declared in model_list", checkChainTiersAreDeclared},
	{"every openrouter/* deployment bounds reasoning", checkOpenRouterReasoningBounds},
	{"no literal api keys", checkNoLiteralAPIKeys},
	{"observability callbacks are declared globally", checkObservabilityCallbacks},
	{"036 did not disturb the failover timing arithmetic", checkTimingArithmeticUnchanged},
	{"every tier of the salary chain declares tool capability", checkToolChainsDeclareCapability},
	{"the embed chain declares consistent output_dimension and input_type", checkEmbedChainDeclarations},
}

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
	for _, key := range toolCapableTaskKeys {
		chain := append([]string{key}, chainFor(c, key)...)
		for _, tier := range chain {
			if !declared[tier] {
				violations = append(violations, fmt.Sprintf(
					"tier %q of tool-using chain %q declares no model_info.supports_function_calling: true; "+
						"a tier added to a tool chain without a capability decision fails the loop's required first round at runtime (contracts/gateway-config.md C3-2)",
					tier, key))
			}
		}
	}
	return violations
}

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

	t.Run("embed chain width matches the application's EMBED_DIMS default", func(t *testing.T) {
		dims, err := appEmbedDims()
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range checkEmbedWidthMatchesAppDefault(cfg, dims) {
			t.Error(v)
		}
	})
}

const validFixture = `
model_list:
  - model_name: match
    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: match-gpt-4o-mini
    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: ghost
    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: ghost-gpt-4o-mini
    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: rephrase
    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: rephrase-deepseek
    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: recruiter
    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: recruiter-gpt-4o-mini
    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: salary
    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
    model_info: {supports_function_calling: true}
  - model_name: salary-claude-haiku
    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
    model_info: {supports_function_calling: true}
  - model_name: outreach
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: outreach-haiku
    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-haiku
    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-analyze
    litellm_params: {model: openrouter/google/gemini-2.5-flash-lite, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-select
    litellm_params: {model: openrouter/google/gemini-2.5-flash-lite, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-select-premium
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-select-premium-haiku
    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-haiku
    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-premium
    litellm_params: {model: openrouter/anthropic/claude-opus-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-premium-sonnet
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-fast
    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-fast-deepseek
    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: embed
    litellm_params: {model: openai/text-embedding-3-small, api_key: os.environ/OPENAI_API_KEY, output_dimension: 1024, input_type: search_document}
litellm_settings:
  success_callback: ["langfuse"]
  failure_callback: ["langfuse"]
  request_timeout: 60
  num_retries: 1
  allowed_fails: 3
  cooldown_time: 60
  fallbacks:
    - match: [match-gpt-4o-mini]
    - ghost: [ghost-gpt-4o-mini]
    - rephrase: [rephrase-deepseek]
    - recruiter: [recruiter-gpt-4o-mini]
    - salary: [salary-claude-haiku]
    - outreach: [outreach-haiku]
    - generation: [generation-haiku]
    - generation-select-premium: [generation-select-premium-haiku]
    - generation-summary: [generation-summary-haiku]
    - generation-summary-premium: [generation-summary-premium-sonnet]
    - generation-summary-fast: [generation-summary-fast-deepseek]
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

	if v := checkEmbedWidthMatchesAppDefault(cfg, 1024); len(v) > 0 {
		t.Errorf("embed width matches app default: valid fixture rejected: %v", v)
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
			name:  "chain dropped for the escalation key",
			check: checkRequestedGroupsHaveChains,
			mutate: func(s string) string {
				return strings.Replace(s, "    - generation-select-premium: [generation-select-premium-haiku]\n", "", 1)
			},
			wantSub: `"generation-select-premium" has no litellm_settings.fallbacks chain`,
		},
		{
			name:  "chain has fewer than two tiers total",
			check: checkChainArityAndProviders,
			mutate: func(s string) string {
				return strings.Replace(s, "    - match: [match-gpt-4o-mini]\n", "    - match: []\n", 1)
			},
			wantSub: "has 1 tier(s) total",
		},
		{
			name:  "a group is renamed to default",
			check: checkNoDefaultOrLocalNaming,
			mutate: func(s string) string {
				return strings.Replace(s, "    - match: [match-gpt-4o-mini]\n", "    - default: [match-gpt-4o-mini]\n", 1)
			},
			wantSub: `is named "default"`,
		},
		{
			name:  "a tier name reintroduces local",
			check: checkNoDefaultOrLocalNaming,
			mutate: func(s string) string {
				s = strings.Replace(s,
					"model_name: match-gpt-4o-mini\n    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}",
					"model_name: match-local\n    litellm_params: {model: openrouter/openai/gpt-4o-mini, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}", 1)
				return strings.Replace(s, "    - match: [match-gpt-4o-mini]\n", "    - match: [match-local]\n", 1)
			},
			wantSub: `"match-local"`,
		},
		{
			name:  "chain names an undeclared tier",
			check: checkChainTiersAreDeclared,
			mutate: func(s string) string {
				return strings.Replace(s, "    - generation-select-premium: [generation-select-premium-haiku]\n", "    - generation-select-premium: [generation-select-premium-typo, generation-select-premium-haiku]\n", 1)
			},
			wantSub: `names tier "generation-select-premium-typo"`,
		},
		{
			name:  "an openrouter tier is left without a reasoning bound",
			check: checkOpenRouterReasoningBounds,
			mutate: func(s string) string {
				return strings.Replace(s,
					"model_name: generation-select-premium\n    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}",
					"model_name: generation-select-premium\n    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, api_key: os.environ/OPENROUTER_API_KEY}", 1)
			},
			wantSub: "declares neither reasoning_effort nor a reasoning block",
		},
		{
			name:  "literal api key",
			check: checkNoLiteralAPIKeys,
			mutate: func(s string) string {
				return strings.Replace(s,
					"model_name: match\n    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}",
					"model_name: match\n    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: sk-real-secret}", 1)
			},
			wantSub: "has a literal api_key",
		},
		{
			name:    "success callback dropped",
			check:   checkObservabilityCallbacks,
			mutate:  func(s string) string { return strings.Replace(s, "  success_callback: [\"langfuse\"]\n", "", 1) },
			wantSub: "success_callback does not list langfuse",
		},
		{

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
					"model_name: match\n    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY}",
					"model_name: match\n    litellm_params: {model: openrouter/deepseek/deepseek-v4-pro, reasoning: {enabled: false}, api_key: os.environ/OPENROUTER_API_KEY, success_callback: [langfuse]}", 1)
			},
			wantSub: "callbacks are global only",
		},
		{

			name:    "request_timeout quietly raised",
			check:   checkTimingArithmeticUnchanged,
			mutate:  func(s string) string { return strings.Replace(s, "request_timeout: 60", "request_timeout: 300", 1) },
			wantSub: "request_timeout is 300, want 60",
		},
		{

			name:  "a tool-chain tier is added without a capability declaration",
			check: checkToolChainsDeclareCapability,
			mutate: func(s string) string {
				return strings.Replace(s,
					"    - salary: [salary-claude-haiku]\n",
					"    - salary: [salary-claude-haiku, salary-mystery]\n", 1)
			},
			wantSub: "declares no model_info.supports_function_calling",
		},
		{
			name:  "a salary tier loses its capability declaration",
			check: checkToolChainsDeclareCapability,
			mutate: func(s string) string {
				return strings.Replace(s,
					"model_name: salary-claude-haiku\n    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}\n    model_info: {supports_function_calling: true}",
					"model_name: salary-claude-haiku\n    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}", 1)
			},
			wantSub: `tier "salary-claude-haiku"`,
		},
		{
			name:  "an embed tier loses its output_dimension",
			check: checkEmbedChainDeclarations,
			mutate: func(s string) string {
				return strings.Replace(s,
					"model_name: embed\n    litellm_params: {model: openai/text-embedding-3-small, api_key: os.environ/OPENAI_API_KEY, output_dimension: 1024, input_type: search_document}",
					"model_name: embed\n    litellm_params: {model: openai/text-embedding-3-small, api_key: os.environ/OPENAI_API_KEY, input_type: search_document}", 1)
			},
			wantSub: "declares no output_dimension",
		},
		{
			name:  "embed width drifts from the app's EMBED_DIMS default",
			check: func(c *gatewayConfig) []string { return checkEmbedWidthMatchesAppDefault(c, 1024) },
			mutate: func(s string) string {
				return strings.Replace(s,
					"model_name: embed\n    litellm_params: {model: openai/text-embedding-3-small, api_key: os.environ/OPENAI_API_KEY, output_dimension: 1024, input_type: search_document}",
					"model_name: embed\n    litellm_params: {model: openai/text-embedding-3-small, api_key: os.environ/OPENAI_API_KEY, output_dimension: 999, input_type: search_document}", 1)
			},
			wantSub: "want 1024",
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
