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

	DatabaseURL string `env:"DATABASE_URL,required"`
	RedisURL    string `env:"REDIS_URL" envDefault:"redis://localhost:6379"`

	// LLM
	LLMProvider string `env:"LLM_PROVIDER" envDefault:"ollama"`
	OllamaURL   string `env:"OLLAMA_URL" envDefault:"http://localhost:11434"`
	LLMModel    string `env:"LLM_MODEL" envDefault:"qwen2.5:14b"`
	EmbedModel  string `env:"EMBED_MODEL" envDefault:"nomic-embed-text"`

	CerebrasAPIKey string `env:"CEREBRAS_API_KEY"`
	CerebrasModel  string `env:"CEREBRAS_MODEL" envDefault:"gpt-oss-120b"`
	CerebrasURL    string `env:"CEREBRAS_URL" envDefault:"https://api.cerebras.ai/v1"`

	EmbedDims                int     `env:"EMBED_DIMS" envDefault:"768"`
	MatchSimilarityThreshold float64 `env:"MATCH_SIMILARITY_THRESHOLD" envDefault:"0.35"`

	ConfigEncryptionKey string `env:"CONFIG_ENCRYPTION_KEY"`

	// Job source credentials
	AdzunaAppID         string `env:"ADZUNA_APP_ID"`
	AdzunaAppKey        string `env:"ADZUNA_APP_KEY"`
	AdzunaCountry       string `env:"ADZUNA_COUNTRY" envDefault:"gb"`
	DjinniSessionCookie string `env:"DJINNI_SESSION_COOKIE"`

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

// Load parses environment variables into a Config, applying defaults for anything unset.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
