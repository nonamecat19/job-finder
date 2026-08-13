package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

// 034 T005: the summary catalogue is Go, the routing it names is YAML, and
// nothing in the compiler connects them.
//
// This test is the connection. An option whose task key the gateway has never
// heard of does not fail loudly — LiteLLM has no such group, the call falls
// through, and the user who paid attention and picked "Premium" silently gets
// whatever the terminal tier is. That is the worst failure mode available here:
// the feature appears to work and quietly does nothing.
//
// It lives in package internal_test rather than in the llm package because it
// spans two layers — the generation domain's catalogue and the platform's
// deployment artifact — and the platform layer must not import generation. The
// existing arch_test.go and toolfence_test.go are here for the same reason.
//
// The parse below is deliberately minimal: only the two things this test
// asserts about. The full config contract is checked by
// internal/platform/llm/gateway_config_test.go, which is also where each of
// these keys is asserted to have a fallback chain across real providers.

type catalogueGatewayConfig struct {
	ModelList []struct {
		ModelName string `yaml:"model_name"`
	} `yaml:"model_list"`
	LiteLLMSettings struct {
		Fallbacks []map[string][]string `yaml:"fallbacks"`
	} `yaml:"litellm_settings"`
}

func TestEverySummaryOptionRoutesSomewhereReal(t *testing.T) {
	// internal/ -> apps/api -> repo root
	path := filepath.Join("..", "..", "..", "gateway", "config.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gateway config: %v", err)
	}
	var cfg catalogueGatewayConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse gateway config: %v", err)
	}

	declared := map[string]bool{}
	for _, d := range cfg.ModelList {
		declared[d.ModelName] = true
	}
	chained := map[string]bool{}
	for _, f := range cfg.LiteLLMSettings.Fallbacks {
		for group := range f {
			chained[group] = true
		}
	}

	checked := 0
	for _, o := range domain.SummaryOptions() {
		checked++
		if o.TaskKey == "" {
			t.Errorf("summary option %q has no task key. Since 044 there is no local tier to fall "+
				"back to, so an option without a key routes nowhere", o.ID)
			continue
		}
		if !declared[o.TaskKey] {
			t.Errorf("summary option %q routes to task key %q, which gateway/config.yaml does not declare. "+
				"The option would appear on the menu and quietly route nowhere", o.ID, o.TaskKey)
		}
		if !chained[o.TaskKey] {
			t.Errorf("summary option %q routes to task key %q, which has no litellm_settings.fallbacks chain. "+
				"Without one a single provider outage takes the option down with it",
				o.ID, o.TaskKey)
		}
	}

	if checked == 0 {
		t.Fatal("no summary options checked; either the catalogue is empty or this test stopped " +
			"finding its options")
	}
}
