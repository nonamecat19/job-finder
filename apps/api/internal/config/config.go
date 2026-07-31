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

	// Gateway (029-litellm-proxy-gateway): an optional LiteLLM proxy that
	// presents one OpenAI-compatible endpoint and routes each task key to a
	// configured provider+model. GatewayURL is the proxy root (empty = gateway
	// unavailable and every gateway task falls back to Ollama). LiteLLMMasterKey
	// authenticates the backend to the proxy; it must be set whenever
	// GatewayURL is.
	GatewayURL       string `mapstructure:"GATEWAY_URL"`
	LiteLLMMasterKey string `mapstructure:"LITELLM_MASTER_KEY"`

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
	// JobLeads credentials: the adapter logs in with these and stores the
	// resulting session cookie in the DB (never in env).
	JobLeadsEmail    string `mapstructure:"JOBLEADS_EMAIL"`
	JobLeadsPassword string `mapstructure:"JOBLEADS_PASSWORD"`
	// Djinni credentials: DjinniSession logs in with these and stores the
	// resulting session cookie in the DB (never in env).
	DjinniEmail    string `mapstructure:"DJINNI_EMAIL"`
	DjinniPassword string `mapstructure:"DJINNI_PASSWORD"`
	JoobleAPIKey   string `mapstructure:"JOOBLE_API_KEY"`
	// DjinniDetailDelayMs is the pause before each detail-page fetch in the
	// enrich queue (concurrency 1), to avoid rate-limiting/banning.
	DjinniDetailDelayMs int `mapstructure:"DJINNI_DETAIL_DELAY_MS"`
	// WorkUaDetailDelayMs is the pause before each work.ua detail-page fetch.
	// 2000 matches work.ua's published Crawl-delay: 2; adapters.WorkUaMinDelay
	// clamps it so a misconfigured env var cannot go below the floor.
	WorkUaDetailDelayMs int `mapstructure:"WORKUA_DETAIL_DELAY_MS"`
	// DjinniRateOverrideRPS pins djinni.co's fetch rate (bypasses the
	// crawl-delay-derived pacing) since its robots.txt Crawl-delay drives the
	// resolver into long cooling-off spirals. 0.5 = one fetch/2s.
	DjinniRateOverrideRPS float64 `mapstructure:"DJINNI_RATE_OVERRIDE_RPS"`

	// Optional services
	FlaresolverrURL string `mapstructure:"FLARESOLVERR_URL"`
	// JobspyURL is the python jobspy sidecar endpoint (apps/jobspy-sidecar),
	// used as JobSpyAdapter's default when a source's runtime config omits it.
	JobspyURL string `mapstructure:"JOBSPY_URL"`

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
	ResumeGroundingLvl string `mapstructure:"RESUME_GROUNDING_LEVEL"`
	RendercvBin        string `mapstructure:"RENDERCV_BIN"`
	// ResumeMasterPath is the dev-fallback master resume YAML path, used when
	// no profile exists yet. Empty falls back to ./resume/resume.yaml.
	ResumeMasterPath string `mapstructure:"RESUME_MASTER_PATH"`

	// Salary inference
	LevelsFyiCSV   string `mapstructure:"LEVELS_FYI_CSV"`
	SalaryFloorUsd int    `mapstructure:"SALARY_FLOOR_USD"`

	// LinkedInScrapeEnabled gates the LinkedIn company-page contact source
	// used by recruiter/hiring-manager resolution (007). Scraping LinkedIn's
	// public pages is a ToS gray area (plan.md Constitution Check), so this
	// defaults to false; enabling it is an explicit operator decision made
	// via env var, not a code change, and is read once at process start.
	LinkedInScrapeEnabled bool `mapstructure:"LINKEDIN_SCRAPE_ENABLED"`

	// AI task concurrency (019-ai-job-throughput). AIConcurrencyCloud applies
	// when a task resolves to a hosted provider (Cerebras or Ollama Cloud);
	// AIConcurrencyLocal applies to a local Ollama and
	// preserves today's behaviour. IngestConcurrency/EnrichConcurrency are
	// non-LLM task pool sizes, promoted from hardcoded literals.
	AIConcurrencyCloud int `mapstructure:"AI_CONCURRENCY_CLOUD"`
	AIConcurrencyLocal int `mapstructure:"AI_CONCURRENCY_LOCAL"`
	IngestConcurrency  int `mapstructure:"INGEST_CONCURRENCY"`
	EnrichConcurrency  int `mapstructure:"ENRICH_CONCURRENCY"`

	// IngestPersistChunkSize is how many postings one ingest run stores per
	// statement batch. Chunks sit inside the run's transaction, so this bounds
	// statement size and lock duration without being a consistency boundary:
	// all chunks of a run commit together, and one failing rolls back the run.
	IngestPersistChunkSize int `mapstructure:"INGEST_PERSIST_CHUNK_SIZE"`

	// Per-task-type deadlines (019-ai-job-throughput). A task exceeding its
	// deadline is finalized `timed_out` rather than hanging indefinitely.
	AITaskTimeoutMatch    time.Duration `mapstructure:"AI_TASK_TIMEOUT_MATCH"`
	AITaskTimeoutGenerate time.Duration `mapstructure:"AI_TASK_TIMEOUT_GENERATE"`
	AITaskTimeoutSalary   time.Duration `mapstructure:"AI_TASK_TIMEOUT_SALARY"`
	AITaskTimeoutGhost    time.Duration `mapstructure:"AI_TASK_TIMEOUT_GHOST"`
	AITaskTimeoutEnrich   time.Duration `mapstructure:"AI_TASK_TIMEOUT_ENRICH"`
	AITaskTimeoutIngest   time.Duration `mapstructure:"AI_TASK_TIMEOUT_INGEST"`

	// Activity liveness / sweeper (019-ai-job-throughput). ActivityStaleAfter
	// must be >= 2x ActivityHeartbeatInterval, and
	// ActivityStaleAfter+ActivitySweepInterval must stay under 5m (FR-009).
	ActivityHeartbeatInterval time.Duration `mapstructure:"ACTIVITY_HEARTBEAT_INTERVAL"`
	ActivityStaleAfter        time.Duration `mapstructure:"ACTIVITY_STALE_AFTER"`
	ActivitySweepInterval     time.Duration `mapstructure:"ACTIVITY_SWEEP_INTERVAL"`
	ActivityQueuedGrace       time.Duration `mapstructure:"ACTIVITY_QUEUED_GRACE"`

	// Local model performance (019-ai-job-throughput). OllamaKeepAlive is
	// sent as `keep_alive` on Ollama chat/embed requests so a local model
	// stays resident; empty omits the field entirely. Ignored by Ollama
	// Cloud. LLMMaxIdleConnsPerHost tunes the LLM clients' idle-connection
	// pool so hosted concurrency doesn't force a fresh TLS handshake per
	// request.
	OllamaKeepAlive        string `mapstructure:"OLLAMA_KEEP_ALIVE"`
	LLMMaxIdleConnsPerHost int    `mapstructure:"LLM_MAX_IDLE_CONNS_PER_HOST"`

	// Database connection capacity (026-db-pool-capacity). Without these the
	// pool is sized by pgx's incidental default, max(4, NumCPU), which is
	// unrelated to how many connections this workload actually holds.

	// DBMaxConns is the pool's maximum size. 0 derives it from total worker
	// concurrency + background allowance + DBInteractiveReserve.
	DBMaxConns int `mapstructure:"DB_MAX_CONNS"`
	// DBMinConns is how many connections stay open while idle, so the first
	// request after a quiet period does not pay a connect round-trip.
	DBMinConns int `mapstructure:"DB_MIN_CONNS"`
	// DBMaxConnLifetime is the age at which a connection is retired regardless
	// of use.
	DBMaxConnLifetime time.Duration `mapstructure:"DB_MAX_CONN_LIFETIME"`
	// DBMaxConnIdleTime is the idle age at which a connection is retired,
	// reclaiming connections silently dropped by intermediaries.
	DBMaxConnIdleTime time.Duration `mapstructure:"DB_MAX_CONN_IDLE_TIME"`
	// DBAcquireTimeout bounds how long an interactive HTTP request waits for a
	// free connection before failing with a capacity error instead of hanging.
	DBAcquireTimeout time.Duration `mapstructure:"DB_ACQUIRE_TIMEOUT"`
	// DBServerMaxConns is Postgres's own max_connections as declared by the
	// operator. Validated against, never queried (research.md R4).
	DBServerMaxConns int `mapstructure:"DB_SERVER_MAX_CONNS"`
	// DBInteractiveReserve is the connections budgeted for dashboard/API
	// traffic above total worker concurrency.
	DBInteractiveReserve int `mapstructure:"DB_INTERACTIVE_RESERVE"`
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
	if err := validateDBPool(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
