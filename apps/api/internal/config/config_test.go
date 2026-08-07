package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port default = %d, want 3000", cfg.Port)
	}
	if cfg.RedisURL != "redis://localhost:6379" {
		t.Errorf("RedisURL default = %q", cfg.RedisURL)
	}
	if cfg.EmbedDims != 768 {
		t.Errorf("EmbedDims default = %d, want 768", cfg.EmbedDims)
	}
	if cfg.MatchSimilarityThreshold != 0.35 {
		t.Errorf("MatchSimilarityThreshold default = %v, want 0.35", cfg.MatchSimilarityThreshold)
	}
	if cfg.OllamaKey != "" {
		t.Errorf("OllamaKey should default empty, got %q", cfg.OllamaKey)
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

func TestLoadLinkedInScrapeEnabledOverride(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINKEDIN_SCRAPE_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
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

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
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

func TestModelOr(t *testing.T) {
	c := &Config{LLMModel: "base"}
	if got := c.ModelOr(""); got != "base" {
		t.Errorf("ModelOr(\"\") = %q, want base", got)
	}
	if got := c.ModelOr("special"); got != "special" {
		t.Errorf("ModelOr(special) = %q, want special", got)
	}
}

func unsetForTest(t *testing.T) error {
	t.Helper()
	all := append([]string{
		"PORT", "REDIS_URL", "OLLAMA_URL", "LLM_MODEL", "EMBED_MODEL", "EMBED_DIMS",
		"MATCH_SIMILARITY_THRESHOLD", "ADZUNA_COUNTRY", "DJINNI_DETAIL_DELAY_MS",
		"WORKUA_DETAIL_DELAY_MS", "DOCUMENTS_DIR",
		"RESUME_GROUNDING_LEVEL", "RENDERCV_BIN", "LINKEDIN_SCRAPE_ENABLED",
	}, optionalKeys...)
	for _, k := range all {
		t.Setenv(k, "")
	}
	return nil
}
