// Package config loads process configuration from environment variables.
// Keys mirror apps/api/.env.example exactly so both backends read the same .env.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

// Config holds every environment-driven setting used across the Go backend.
// Field names map 1:1 to apps/api/.env.example keys via the `env` tag.
type Config struct {
	Port int `env:"PORT" envDefault:"3000"`

	// Not `,required` at the struct level: some binaries (e.g. cmd/llmsmoke)
	// don't touch the database. cmd/server validates this is set before use.
	DatabaseURL string `env:"DATABASE_URL"`
	RedisURL    string `env:"REDIS_URL" envDefault:"redis://localhost:6379"`

	// LLM. The only provider is Ollama; OllamaKey authenticates to Ollama Cloud
	// (https://ollama.com) via a Bearer header, and is empty for a local server.
	OllamaURL  string `env:"OLLAMA_URL" envDefault:"http://localhost:11434"`
	OllamaKey  string `env:"OLLAMA_KEY"`
	LLMModel   string `env:"LLM_MODEL" envDefault:"qwen2.5:14b"`
	EmbedModel string `env:"EMBED_MODEL" envDefault:"nomic-embed-text"`

	// Per-task chat models. Empty falls back to LLMModel via ModelOr.
	LLMModelMatch      string `env:"LLM_MODEL_MATCH"`
	LLMModelGeneration string `env:"LLM_MODEL_GENERATION"`

	// EmbedURL is the endpoint for embeddings; empty means "same as OllamaURL".
	// Ollama Cloud serves no embedding models, so this can point at a local
	// Ollama (http://localhost:11434) while chat stays on the cloud.
	EmbedURL string `env:"EMBED_URL"`

	EmbedDims                int     `env:"EMBED_DIMS" envDefault:"768"`
	MatchSimilarityThreshold float64 `env:"MATCH_SIMILARITY_THRESHOLD" envDefault:"0.35"`

	ConfigEncryptionKey string `env:"CONFIG_ENCRYPTION_KEY"`

	// Job source credentials
	AdzunaAppID         string `env:"ADZUNA_APP_ID"`
	AdzunaAppKey        string `env:"ADZUNA_APP_KEY"`
	AdzunaCountry       string `env:"ADZUNA_COUNTRY" envDefault:"gb"`
	DjinniSessionCookie string `env:"DJINNI_SESSION_COOKIE"`
	JoobleAPIKey        string `env:"JOOBLE_API_KEY"`
	// DjinniDetailDelayMs is the pause before each detail-page fetch in the
	// enrich queue (concurrency 1), to avoid rate-limiting/banning the
	// authenticated djinni account.
	DjinniDetailDelayMs int `env:"DJINNI_DETAIL_DELAY_MS" envDefault:"1500"`
	// WorkUaDetailDelayMs is the pause before each work.ua detail-page fetch.
	// 2000 matches work.ua's published Crawl-delay: 2; adapters.WorkUaMinDelay
	// clamps it so a misconfigured env var cannot go below the floor.
	WorkUaDetailDelayMs int `env:"WORKUA_DETAIL_DELAY_MS" envDefault:"2000"`

	// Sidecar / optional services
	JobspyURL       string `env:"JOBSPY_URL" envDefault:"http://localhost:8000"`
	FlaresolverrURL string `env:"FLARESOLVERR_URL"`

	// Paths
	DocumentsDir string `env:"DOCUMENTS_DIR" envDefault:"/data/documents"`

	// RenderCV
	ResumeMasterPath   string `env:"RESUME_MASTER_PATH" envDefault:"./resume/resume.yaml"`
	ResumeGroundingLvl string `env:"RESUME_GROUNDING_LEVEL" envDefault:"moderate"`
	RendercvBin        string `env:"RENDERCV_BIN" envDefault:"rendercv"`
}

// ModelOr returns m if set, otherwise the default LLMModel. Used to resolve a
// per-task model env var that is allowed to be empty.
func (c *Config) ModelOr(m string) string {
	if m == "" {
		return c.LLMModel
	}
	return m
}

// Load parses environment variables into a Config, applying defaults for anything unset.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
