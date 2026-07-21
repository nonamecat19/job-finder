// Package config loads process configuration from environment variables using
// viper. Keys mirror apps/api/.env.example exactly so both backends read the
// same .env. Viper gives us layered precedence (env > code defaults) today and
// a clean seam to add a config file later without touching call sites.
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config holds every environment-driven setting used across the Go backend.
// Field names map 1:1 to apps/api/.env.example keys via the `mapstructure` tag
// (which equals the env var name, since viper binds each key to its env var).
type Config struct {
	Port int `mapstructure:"PORT"`

	// Not required here: some binaries (e.g. cmd/llmsmoke) don't touch the
	// database. cmd/server validates this is set before use.
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	RedisURL    string `mapstructure:"REDIS_URL"`

	// LLM. The only provider is Ollama; OllamaKey authenticates to Ollama Cloud
	// (https://ollama.com) via a Bearer header, and is empty for a local server.
	OllamaURL  string `mapstructure:"OLLAMA_URL"`
	OllamaKey  string `mapstructure:"OLLAMA_KEY"`
	LLMModel   string `mapstructure:"LLM_MODEL"`
	EmbedModel string `mapstructure:"EMBED_MODEL"`

	// Per-task chat models. Empty falls back to LLMModel via ModelOr.
	LLMModelMatch      string `mapstructure:"LLM_MODEL_MATCH"`
	LLMModelGeneration string `mapstructure:"LLM_MODEL_GENERATION"`
	// LLMModelRephrase is the model for the keyword-diff rephrase suggester
	// (008-5). Empty falls back to LLMModel via ModelOr.
	LLMModelRephrase string `mapstructure:"LLM_MODEL_REPHRASE"`

	// KeywordRephraseCacheTTLSec is the lifetime, in seconds, of a cached set of
	// keyword-diff rephrase suggestions. Suggestions are generated async and
	// cached because each is a live LLM call; a stale entry past this age is
	// recomputed on the next request.
	KeywordRephraseCacheTTLSec int `mapstructure:"KEYWORD_REPHRASE_CACHE_TTL_SEC"`

	// EmbedURL is the endpoint for embeddings; empty means "same as OllamaURL".
	// Ollama Cloud serves no embedding models, so this can point at a local
	// Ollama (http://localhost:11434) while chat stays on the cloud.
	EmbedURL string `mapstructure:"EMBED_URL"`

	EmbedDims                int     `mapstructure:"EMBED_DIMS"`
	MatchSimilarityThreshold float64 `mapstructure:"MATCH_SIMILARITY_THRESHOLD"`

	// Fresh-match notification (010-4)
	MatchNotifyScoreThreshold int `mapstructure:"MATCH_NOTIFY_SCORE_THRESHOLD"`
	MatchNotifyRateLimit      int `mapstructure:"MATCH_NOTIFY_RATE_LIMIT"`

	ConfigEncryptionKey string `mapstructure:"CONFIG_ENCRYPTION_KEY"`

	// Job source credentials
	AdzunaAppID   string `mapstructure:"ADZUNA_APP_ID"`
	AdzunaAppKey  string `mapstructure:"ADZUNA_APP_KEY"`
	AdzunaCountry string `mapstructure:"ADZUNA_COUNTRY"`
	// Djinni credentials: the adapter logs in with these and stores the
	// resulting sessionid cookie in the DB (never in env).
	DjinniEmail    string `mapstructure:"DJINNI_EMAIL"`
	DjinniPassword string `mapstructure:"DJINNI_PASSWORD"`
	JoobleAPIKey   string `mapstructure:"JOOBLE_API_KEY"`
	// DjinniDetailDelayMs is the pause before each detail-page fetch in the
	// enrich queue (concurrency 1), to avoid rate-limiting/banning the
	// authenticated djinni account.
	DjinniDetailDelayMs int `mapstructure:"DJINNI_DETAIL_DELAY_MS"`
	// WorkUaDetailDelayMs is the pause before each work.ua detail-page fetch.
	// 2000 matches work.ua's published Crawl-delay: 2; adapters.WorkUaMinDelay
	// clamps it so a misconfigured env var cannot go below the floor.
	WorkUaDetailDelayMs int `mapstructure:"WORKUA_DETAIL_DELAY_MS"`

	// Sidecar / optional services
	JobspyURL       string `mapstructure:"JOBSPY_URL"`
	FlaresolverrURL string `mapstructure:"FLARESOLVERR_URL"`

	// Paths
	DocumentsDir string `mapstructure:"DOCUMENTS_DIR"`

	// MinIO object storage for generated resume/cover-letter files. An empty
	// MinioEndpoint disables uploads (files stay on DocumentsDir only).
	MinioEndpoint  string `mapstructure:"MINIO_ENDPOINT"`
	MinioAccessKey string `mapstructure:"MINIO_ACCESS_KEY"`
	MinioSecretKey string `mapstructure:"MINIO_SECRET_KEY"`
	MinioBucket    string `mapstructure:"MINIO_BUCKET"`
	MinioUseSSL    bool   `mapstructure:"MINIO_USE_SSL"`

	// RenderCV
	ResumeMasterPath   string `mapstructure:"RESUME_MASTER_PATH"`
	ResumeGroundingLvl string `mapstructure:"RESUME_GROUNDING_LEVEL"`
	RendercvBin        string `mapstructure:"RENDERCV_BIN"`

	// Salary inference
	LevelsFyiCSV  string `mapstructure:"LEVELS_FYI_CSV"`
	SalaryFloorUsd int   `mapstructure:"SALARY_FLOOR_USD"`
}

// defaults holds the code-level default for every key that has one. Keys
// without an entry default to the zero value (and, where required, are
// validated by the consuming binary).
var defaults = map[string]any{
	"PORT":                           3000,
	"REDIS_URL":                      "redis://localhost:6379",
	"OLLAMA_URL":                     "http://localhost:11434",
	"LLM_MODEL":                      "qwen2.5:14b",
	"EMBED_MODEL":                    "nomic-embed-text",
	"EMBED_DIMS":                     768,
	"MATCH_SIMILARITY_THRESHOLD":     0.35,
	"KEYWORD_REPHRASE_CACHE_TTL_SEC": 900,
	"ADZUNA_COUNTRY":                 "gb",
	"DJINNI_DETAIL_DELAY_MS":         1500,
	"WORKUA_DETAIL_DELAY_MS":         2000,
	"JOBSPY_URL":                     "http://localhost:8000",
	// "/data/documents" is a container-only path (writable there because the
	// Dockerfile/compose files run the API as root with a dedicated volume).
	// On bare-metal dev or `go test`, the host user can't mkdir /data at all
	// ("mkdir /data: permission denied") — default to a repo-relative dir
	// instead; docker-compose.yml / docker-compose.prod.yml / Dockerfile all
	// set DOCUMENTS_DIR=/data/documents explicitly, so containers are unaffected.
	"DOCUMENTS_DIR":          "./data/documents",
	"MINIO_BUCKET":           "documents",
	"MINIO_USE_SSL":          false,
	"RESUME_MASTER_PATH":     "./resume/resume.yaml",
	"RESUME_GROUNDING_LEVEL": "moderate",
	"RENDERCV_BIN":           "rendercv",
}

// keys without a default (optional strings / required-by-consumer). Listed so
// viper binds each to its env var even when unset — mapstructure only sees keys
// viper knows about.
var optionalKeys = []string{
	"DATABASE_URL", "OLLAMA_KEY", "LLM_MODEL_MATCH", "LLM_MODEL_GENERATION",
	"LLM_MODEL_REPHRASE",
	"EMBED_URL", "CONFIG_ENCRYPTION_KEY", "ADZUNA_APP_ID", "ADZUNA_APP_KEY",
	"DJINNI_EMAIL", "DJINNI_PASSWORD", "JOOBLE_API_KEY", "FLARESOLVERR_URL",
	"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY",
	"LEVELS_FYI_CSV", "SALARY_FLOOR_USD",
}

// ModelOr returns m if set, otherwise the default LLMModel. Used to resolve a
// per-task model env var that is allowed to be empty.
func (c *Config) ModelOr(m string) string {
	if m == "" {
		return c.LLMModel
	}
	return m
}

// Load reads environment variables into a Config via viper, applying code
// defaults for anything unset.
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()

	for k, val := range defaults {
		v.SetDefault(k, val)
	}
	// Bind keys that have no default so Unmarshal still reads their env var.
	for _, k := range optionalKeys {
		if err := v.BindEnv(k); err != nil {
			return nil, fmt.Errorf("config: bind %s: %w", k, err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
