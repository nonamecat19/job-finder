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

// TestConfigRequiresGateway guards K1: Load() (used by binaries that do
// inference work) must fail closed when the gateway is not configured, and
// the error must name the specific missing key — the same shape
// queue.validateLiveness uses. It must never attempt to reach the network:
// every case here runs with no server listening anywhere, and if Load()
// tried an HTTP call the missing-key or bad-URL cases would hang or error on
// the network rather than failing fast with a config error.
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

// TestLoadNonAIIgnoresGateway guards K1-4: the sibling loader for binaries and
// tests that do no inference must succeed with no gateway configured at all.
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

// TestApiContainerHoldsNoProviderCredentials guards Constitution Principle V's
// credential clause: provider credentials must stay in the gateway container
// and must not be readable through the application.
//
// This exists because the prod api service previously carried `env_file: .env`,
// which handed it every provider key. That was a standing violation, not a
// hypothetical — anyone with a shell in the app container could read
// OPENROUTER_API_KEY. The allowlist that replaced it only stays correct if
// something fails when a key is added back.
//
// LITELLM_MASTER_KEY is deliberately permitted: it authenticates the app TO the
// gateway and is not a provider credential.
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
			continue // dev compose has no api service; the process runs on the host
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

// TestGatewayDoesNotDependOnTheCollector is the FR-004 gate expressed as
// something that fails (036 contracts C3-2).
//
// The collector must never be able to hold inference up. A `depends_on` for any
// collector service would mean a broken or slow-starting ClickHouse delays the
// gateway, which delays every AI task in the platform — for a service whose
// entire job is to watch. The rule is easy to violate by accident when adding
// startup ordering, so it is asserted rather than remembered.
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
			continue // prod has no gateway; see the header comment in that file
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

// TestCollectorUIIsBoundToLoopback guards contracts C3-4. The collector's UI
// holds the user's legal name, employers and contact details in plain text, so
// a published port on 0.0.0.0 exposes the profile to the whole network — in dev
// as much as in prod.
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
			continue // prod has no collector; see the header comment in that file
		}
		if !strings.Contains(web, "ports:") {
			continue // not published at all is stricter still
		}
		if !strings.Contains(web, "127.0.0.1:") {
			t.Errorf("%s: langfuse-web publishes a port without a loopback bind; the operator UI holds the user's full profile in plain text", file)
		}
	}
}

// composeService returns the block of a compose file belonging to one service,
// with comment lines stripped so a key named in a comment is not mistaken for a
// key being granted.
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
			// A new top-level service starts at exactly two spaces of indent.
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
