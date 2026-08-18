package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

type catalogueGatewayConfig struct {
	ModelList []struct {
		ModelName string `yaml:"model_name"`
	} `yaml:"model_list"`
	LiteLLMSettings struct {
		Fallbacks []map[string][]string `yaml:"fallbacks"`
	} `yaml:"litellm_settings"`
}

func TestEverySummaryOptionRoutesSomewhereReal(t *testing.T) {

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
