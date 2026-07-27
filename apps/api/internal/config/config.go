// Package config loads process configuration from environment variables using
// viper. Keys mirror apps/api/.env.example exactly so both backends read the
// same .env. Viper gives us layered precedence (env > code defaults) today and
// a clean seam to add a config file later without touching call sites.
package config

import (
	"fmt"
	"time"

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
	// LLMModelGhost is the model for the ghost-job detector (005). Empty
	// falls back to LLMModel via ModelOr.
	LLMModelGhost string `mapstructure:"LLM_MODEL_GHOST"`

	// Cerebras (001-cerebras-model-toggle): an optional second chat provider,
	// selectable per task from dashboard Settings. CerebrasAPIKey is a secret
	// with no default; when empty, Cerebras is unavailable and every task
	// resolves to Ollama regardless of its persisted setting. CerebrasBaseURL
	// defaults to the public Cerebras API. Cerebras has no embeddings endpoint,
	// so EmbedURL/EmbedModel above are unaffected by this provider.
	CerebrasAPIKey  string `mapstructure:"CEREBRAS_API_KEY"`
	CerebrasBaseURL string `mapstructure:"CEREBRAS_BASE_URL"`

	// OpenRouter: an optional third chat provider, selectable per task from
	// dashboard Settings exactly like Cerebras. OpenRouterAPIKey is a secret
	// with no default; when empty, OpenRouter is unavailable and tasks set to
	// it resolve to Ollama. OpenRouterBaseURL defaults to the public API.
	// OpenRouterSiteURL/OpenRouterAppName are optional attribution headers
	// (HTTP-Referer / X-Title) OpenRouter uses for leaderboard ranking.
	// OpenRouter has no embeddings endpoint, so EmbedURL/EmbedModel are
	// unaffected by this provider.
	OpenRouterAPIKey  string `mapstructure:"OPENROUTER_API_KEY"`
	OpenRouterBaseURL string `mapstructure:"OPENROUTER_BASE_URL"`
	OpenRouterSiteURL string `mapstructure:"OPENROUTER_SITE_URL"`
	OpenRouterAppName string `mapstructure:"OPENROUTER_APP_NAME"`

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

	// ExtJWTSecret signs the browser-extension access token (014-autofill-
	// extension). A 32-byte hex string (openssl rand -hex 32), same shape as
	// ConfigEncryptionKey. If unset, cmd/server generates a random ephemeral
	// secret at startup and logs a warning — tokens then stop validating
	// across restarts, which is safer than a hardcoded default secret.
	ExtJWTSecret string `mapstructure:"EXT_JWT_SECRET"`

	// Job source credentials
	AdzunaAppID   string `mapstructure:"ADZUNA_APP_ID"`
	AdzunaAppKey  string `mapstructure:"ADZUNA_APP_KEY"`
	AdzunaCountry string `mapstructure:"ADZUNA_COUNTRY"`
	// JobLeads credentials: the adapter logs in with these and stores the
	// resulting session cookie in the DB (never in env).
	JobLeadsEmail    string `mapstructure:"JOBLEADS_EMAIL"`
	JobLeadsPassword string `mapstructure:"JOBLEADS_PASSWORD"`
	JoobleAPIKey     string `mapstructure:"JOOBLE_API_KEY"`
	// DjinniDetailDelayMs is the pause before each detail-page fetch in the
	// enrich queue (concurrency 1), to avoid rate-limiting/banning.
	DjinniDetailDelayMs int `mapstructure:"DJINNI_DETAIL_DELAY_MS"`
	// WorkUaDetailDelayMs is the pause before each work.ua detail-page fetch.
	// 2000 matches work.ua's published Crawl-delay: 2; adapters.WorkUaMinDelay
	// clamps it so a misconfigured env var cannot go below the floor.
	WorkUaDetailDelayMs int `mapstructure:"WORKUA_DETAIL_DELAY_MS"`

	// Optional services
	FlaresolverrURL string `mapstructure:"FLARESOLVERR_URL"`

	// Browser identity version — bumped whenever the UA/header/TLS profile changes.
	BrowserIdentityVersion string `mapstructure:"BROWSER_IDENTITY_VERSION"`

	// CoolingOffThreshold is the consecutive block count at which cooling-off
	// kicks in (FR-026).
	CoolingOffThreshold int `mapstructure:"COOLING_OFF_THRESHOLD"`

	// CoolingOffBaseDuration is the base duration for the first cooling-off
	// period; it doubles on each consecutive extension (FR-026).
	CoolingOffBaseDuration time.Duration `mapstructure:"COOLING_OFF_BASE_DURATION"`

	// CheapRungRetestInterval is how often a host pinned to an expensive rung
	// re-tests the cheap rung (FR-014).
	CheapRungRetestInterval time.Duration `mapstructure:"CHEAP_RUNG_RETEST_INTERVAL"`

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
	LevelsFyiCSV   string `mapstructure:"LEVELS_FYI_CSV"`
	SalaryFloorUsd int    `mapstructure:"SALARY_FLOOR_USD"`

	// LinkedInScrapeEnabled gates the LinkedIn company-page contact source
	// used by recruiter/hiring-manager resolution (007). Scraping LinkedIn's
	// public pages is a ToS gray area (plan.md Constitution Check), so this
	// defaults to false; enabling it is an explicit operator decision made
	// via env var, not a code change, and is read once at process start.
	LinkedInScrapeEnabled bool `mapstructure:"LINKEDIN_SCRAPE_ENABLED"`
}

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
