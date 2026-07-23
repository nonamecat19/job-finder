package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	// t.Setenv on an unrelated key ensures a clean, isolated env for the test
	// process without clobbering the real environment.
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
	if cfg.CerebrasAPIKey != "" {
		t.Errorf("CerebrasAPIKey should default empty, got %q", cfg.CerebrasAPIKey)
	}
	if cfg.CerebrasBaseURL != "https://api.cerebras.ai/v1" {
		t.Errorf("CerebrasBaseURL default = %q, want https://api.cerebras.ai/v1", cfg.CerebrasBaseURL)
	}
}

func TestLoadCerebrasAPIKeyOverride(t *testing.T) {
	if err := unsetForTest(t); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CEREBRAS_API_KEY", "csk-test")
	t.Setenv("CEREBRAS_BASE_URL", "http://localhost:9999/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CerebrasAPIKey != "csk-test" {
		t.Errorf("CerebrasAPIKey = %q, want csk-test", cfg.CerebrasAPIKey)
	}
	if cfg.CerebrasBaseURL != "http://localhost:9999/v1" {
		t.Errorf("CerebrasBaseURL = %q, want http://localhost:9999/v1", cfg.CerebrasBaseURL)
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

// unsetForTest clears every config env var for the duration of the test, so a
// developer's real .env / shell exports don't leak into default assertions.
func unsetForTest(t *testing.T) error {
	t.Helper()
	all := append([]string{
		"PORT", "REDIS_URL", "OLLAMA_URL", "LLM_MODEL", "EMBED_MODEL", "EMBED_DIMS",
		"MATCH_SIMILARITY_THRESHOLD", "ADZUNA_COUNTRY", "DJINNI_DETAIL_DELAY_MS",
		"WORKUA_DETAIL_DELAY_MS", "JOBSPY_URL", "DOCUMENTS_DIR", "RESUME_MASTER_PATH",
		"RESUME_GROUNDING_LEVEL", "RENDERCV_BIN", "LINKEDIN_SCRAPE_ENABLED",
	}, optionalKeys...)
	for _, k := range all {
		t.Setenv(k, "")
	}
	return nil
}
