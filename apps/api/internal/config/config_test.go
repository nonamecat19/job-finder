package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadNonAI()
	if err != nil {
		t.Fatalf("LoadNonAI: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port default = %d, want 3000", cfg.Port)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("RedisURL default = %q", cfg.RedisURL)
	}
	if cfg.EmbedDims != 1024 {
		t.Errorf("EmbedDims default = %d, want 1024", cfg.EmbedDims)
	}
	if cfg.MatchSimilarityThreshold != 0.35 {
		t.Errorf("MatchSimilarityThreshold default = %v, want 0.35", cfg.MatchSimilarityThreshold)
	}
	if cfg.LinkedInScrapeEnabled {
		t.Error("LinkedInScrapeEnabled should default false")
	}
	if cfg.GatewayURL != "" {
		t.Errorf("GatewayURL should default empty, got %q", cfg.GatewayURL)
	}
	if cfg.LiteLLMMasterKey != "" {
		t.Errorf("LiteLLMMasterKey should default empty, got %q", cfg.LiteLLMMasterKey)
	}
}

func TestLoadGatewayOverride(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_URL", "http://litellm:4000")
	t.Setenv("LITELLM_MASTER_KEY", "sk-test")
	t.Setenv("RABBITMQ_URL", "amqp://jobfinder:test@localhost:5672/")
	t.Setenv("AI_SERVICE_URL", "http://ai:8000")
	t.Setenv("AI_SERVICE_TOKEN", "shared-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GatewayURL != "http://litellm:4000" {
		t.Errorf("GatewayURL = %q, want http://litellm:4000", cfg.GatewayURL)
	}
	if cfg.LiteLLMMasterKey != "sk-test" {
		t.Errorf("LiteLLMMasterKey = %q, want sk-test", cfg.LiteLLMMasterKey)
	}
}

func TestConfigRequiresGateway(t *testing.T) {
	cases := []struct {
		name       string
		gatewayURL string
		masterKey  string
		wantKey    string
	}{
		{name: "both unset", gatewayURL: "", masterKey: "", wantKey: "GATEWAY_URL"},
		{name: "master key unset", gatewayURL: "http://litellm:4000", masterKey: "", wantKey: "LITELLM_MASTER_KEY"},
		{name: "gateway url unset", gatewayURL: "", masterKey: "sk-test", wantKey: "GATEWAY_URL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unsetForTest(t); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GATEWAY_URL", tc.gatewayURL)
			t.Setenv("LITELLM_MASTER_KEY", tc.masterKey)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load: want error naming %s, got nil", tc.wantKey)
			}
			if !strings.Contains(err.Error(), tc.wantKey) {
				t.Errorf("Load error = %q, want it to name %s", err.Error(), tc.wantKey)
			}
		})
	}
}

func TestLoadNonAIIgnoresGateway(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadNonAI()
	if err != nil {
		t.Fatalf("LoadNonAI: %v", err)
	}
	if cfg.GatewayURL != "" {
		t.Errorf("GatewayURL = %q, want empty", cfg.GatewayURL)
	}
	if cfg.LiteLLMMasterKey != "" {
		t.Errorf("LiteLLMMasterKey = %q, want empty", cfg.LiteLLMMasterKey)
	}
}

func TestLoadLinkedInScrapeEnabledOverride(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINKEDIN_SCRAPE_ENABLED", "true")

	cfg, err := LoadNonAI()
	if err != nil {
		t.Fatalf("LoadNonAI: %v", err)
	}
	if !cfg.LinkedInScrapeEnabled {
		t.Error("LinkedInScrapeEnabled = false, want true after env override")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBED_DIMS", "1024")
	t.Setenv("MATCH_SIMILARITY_THRESHOLD", "0.5")

	cfg, err := LoadNonAI()
	if err != nil {
		t.Fatalf("LoadNonAI: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://x" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.EmbedDims != 1024 {
		t.Errorf("EmbedDims = %d, want 1024", cfg.EmbedDims)
	}
	if cfg.MatchSimilarityThreshold != 0.5 {
		t.Errorf("MatchSimilarityThreshold = %v, want 0.5", cfg.MatchSimilarityThreshold)
	}
}

func TestCapabilityRoutingDefaultsToGo(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_URL", "http://litellm:4000")
	t.Setenv("LITELLM_MASTER_KEY", "sk-test")
	t.Setenv("RABBITMQ_URL", "amqp://jobfinder:test@localhost:5672/")
	t.Setenv("AI_SERVICE_URL", "http://ai:8000")
	t.Setenv("AI_SERVICE_TOKEN", "shared-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.CapabilityRouting("ghost"); got != "go" {
		t.Errorf("CapabilityRouting(ghost) = %q, want go", got)
	}
}

func TestCapabilityRoutingParsesPairs(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_URL", "http://litellm:4000")
	t.Setenv("LITELLM_MASTER_KEY", "sk-test")
	t.Setenv("RABBITMQ_URL", "amqp://jobfinder:test@localhost:5672/")
	t.Setenv("AI_SERVICE_URL", "http://ai:8000")
	t.Setenv("AI_SERVICE_TOKEN", "shared-secret")
	t.Setenv("AI_CAPABILITY_ROUTING", "ghost=python, match=go")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.CapabilityRouting("ghost"); got != "python" {
		t.Errorf("CapabilityRouting(ghost) = %q, want python", got)
	}
	if got := cfg.CapabilityRouting("match"); got != "go" {
		t.Errorf("CapabilityRouting(match) = %q, want go", got)
	}
	if got := cfg.CapabilityRouting("salary"); got != "go" {
		t.Errorf("CapabilityRouting(salary) = %q, want go (unlisted)", got)
	}
}

func TestCapabilityRoutingRejectsMalformedEntry(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_URL", "http://litellm:4000")
	t.Setenv("LITELLM_MASTER_KEY", "sk-test")
	t.Setenv("RABBITMQ_URL", "amqp://jobfinder:test@localhost:5672/")
	t.Setenv("AI_CAPABILITY_ROUTING", "ghost=maybe")

	_, err := Load()
	if err == nil {
		t.Fatal("Load: want error for invalid mode, got nil")
	}
	if !strings.Contains(err.Error(), "ghost=maybe") {
		t.Errorf("Load error = %q, want it to name the malformed entry", err.Error())
	}
}

func TestAIServiceCredentialsRequired(t *testing.T) {
	cases := []struct {
		name    string
		routing string
		url     string
		token   string
		wantKey string
	}{
		{name: "no routing entry at all, both unset", routing: "", url: "", token: "", wantKey: "AI_SERVICE_URL"},
		{name: "explicit python routing, both unset", routing: "ghost=python", url: "", token: "", wantKey: "AI_SERVICE_URL"},
		{name: "explicit python routing, token unset", routing: "ghost=python", url: "http://ai:8000", token: "", wantKey: "AI_SERVICE_TOKEN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := unsetForTest(t); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GATEWAY_URL", "http://litellm:4000")
			t.Setenv("LITELLM_MASTER_KEY", "sk-test")
			t.Setenv("RABBITMQ_URL", "amqp://jobfinder:test@localhost:5672/")
			t.Setenv("AI_CAPABILITY_ROUTING", tc.routing)
			t.Setenv("AI_SERVICE_URL", tc.url)
			t.Setenv("AI_SERVICE_TOKEN", tc.token)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load: want error naming %s, got nil", tc.wantKey)
			}
			if !strings.Contains(err.Error(), tc.wantKey) {
				t.Errorf("Load error = %q, want it to name %s", err.Error(), tc.wantKey)
			}
		})
	}
}

func unsetForTest(t *testing.T) error {
	t.Helper()
	all := append([]string{
		"PORT", "REDIS_URL", "EMBED_DIMS",
		"MATCH_SIMILARITY_THRESHOLD", "ADZUNA_COUNTRY", "DJINNI_DETAIL_DELAY_MS",
		"WORKUA_DETAIL_DELAY_MS", "DOCUMENTS_DIR",
		"RESUME_GROUNDING_LEVEL", "RENDERCV_BIN", "LINKEDIN_SCRAPE_ENABLED",
	}, optionalKeys...)
	for _, k := range all {
		t.Setenv(k, "")
	}
	return nil
}

func TestApiContainerHoldsNoProviderCredentials(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	for _, file := range []string{"docker-compose.prod.yml", "docker-compose.yml"} {
		path := filepath.Join(root, file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		api, ok := composeService(string(data), "api")
		if !ok {
			continue
		}
		if strings.Contains(api, "env_file") {
			t.Errorf("%s: api service uses env_file, which imports every variable including provider credentials", file)
		}
		for _, key := range []string{
			"GROQ_API_KEY", "COHERE_API_KEY", "OPENROUTER_API_KEY",
			"LANGFUSE_SECRET_KEY", "LANGFUSE_PUBLIC_KEY",
		} {
			if strings.Contains(api, key) {
				t.Errorf("%s: api service is granted %s; provider and collector credentials must not be readable from the application", file, key)
			}
		}
	}
}

func TestGatewayDoesNotDependOnTheCollector(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	collector := []string{"langfuse-web", "langfuse-worker", "clickhouse"}

	for _, file := range []string{"docker-compose.yml", "docker-compose.prod.yml"} {
		path := filepath.Join(root, file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		litellm, ok := composeService(string(data), "litellm")
		if !ok {
			continue
		}
		if !strings.Contains(litellm, "depends_on") {
			continue
		}
		for _, svc := range collector {
			if strings.Contains(litellm, svc) {
				t.Errorf("%s: litellm depends_on %s; a collector that can delay the gateway can delay every AI task (FR-004)", file, svc)
			}
		}
	}
}

func TestCollectorUIIsBoundToLoopback(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	for _, file := range []string{"docker-compose.yml", "docker-compose.prod.yml"} {
		path := filepath.Join(root, file)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		web, ok := composeService(string(data), "langfuse-web")
		if !ok {
			continue
		}
		if !strings.Contains(web, "ports:") {
			continue
		}
		if !strings.Contains(web, "127.0.0.1:") {
			t.Errorf("%s: langfuse-web publishes a port without a loopback bind; the operator UI holds the user's full profile in plain text", file)
		}
	}
}

func composeService(doc, name string) (string, bool) {
	lines := strings.Split(doc, "\n")
	var out []string
	in := false
	for _, l := range lines {
		if strings.HasPrefix(l, "  "+name+":") {
			in = true
			continue
		}
		if in {

			if len(l) > 2 && l[0] == ' ' && l[1] == ' ' && l[2] != ' ' && strings.HasSuffix(strings.TrimSpace(l), ":") {
				break
			}
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n"), in
}
