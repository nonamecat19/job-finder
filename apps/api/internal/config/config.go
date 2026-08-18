package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port int `mapstructure:"PORT"`

	DatabaseURL string `mapstructure:"DATABASE_URL"`
	RedisURL    string `mapstructure:"REDIS_URL"`
	RabbitMQURL string `mapstructure:"RABBITMQ_URL"`

	GatewayURL       string `mapstructure:"GATEWAY_URL"`
	LiteLLMMasterKey string `mapstructure:"LITELLM_MASTER_KEY"`

	KeywordRephraseCacheTTLSec int `mapstructure:"KEYWORD_REPHRASE_CACHE_TTL_SEC"`

	EmbedDims                int     `mapstructure:"EMBED_DIMS"`
	MatchSimilarityThreshold float64 `mapstructure:"MATCH_SIMILARITY_THRESHOLD"`

	MatchNotifyScoreThreshold int `mapstructure:"MATCH_NOTIFY_SCORE_THRESHOLD"`
	MatchNotifyRateLimit      int `mapstructure:"MATCH_NOTIFY_RATE_LIMIT"`

	ConfigEncryptionKey string `mapstructure:"CONFIG_ENCRYPTION_KEY"`

	AdzunaAppID           string  `mapstructure:"ADZUNA_APP_ID"`
	AdzunaAppKey          string  `mapstructure:"ADZUNA_APP_KEY"`
	AdzunaCountry         string  `mapstructure:"ADZUNA_COUNTRY"`
	JobLeadsEmail         string  `mapstructure:"JOBLEADS_EMAIL"`
	JobLeadsPassword      string  `mapstructure:"JOBLEADS_PASSWORD"`
	DjinniEmail           string  `mapstructure:"DJINNI_EMAIL"`
	DjinniPassword        string  `mapstructure:"DJINNI_PASSWORD"`
	JoobleAPIKey          string  `mapstructure:"JOOBLE_API_KEY"`
	DjinniDetailDelayMs   int     `mapstructure:"DJINNI_DETAIL_DELAY_MS"`
	WorkUaDetailDelayMs   int     `mapstructure:"WORKUA_DETAIL_DELAY_MS"`
	DjinniRateOverrideRPS float64 `mapstructure:"DJINNI_RATE_OVERRIDE_RPS"`

	FlaresolverrURL string `mapstructure:"FLARESOLVERR_URL"`
	JobspyURL       string `mapstructure:"JOBSPY_URL"`

	BrowserIdentityVersion string `mapstructure:"BROWSER_IDENTITY_VERSION"`

	CoolingOffThreshold int `mapstructure:"COOLING_OFF_THRESHOLD"`

	CoolingOffBaseDuration time.Duration `mapstructure:"COOLING_OFF_BASE_DURATION"`

	CheapRungRetestInterval time.Duration `mapstructure:"CHEAP_RUNG_RETEST_INTERVAL"`

	DocumentsDir string `mapstructure:"DOCUMENTS_DIR"`

	MinioEndpoint  string `mapstructure:"MINIO_ENDPOINT"`
	MinioAccessKey string `mapstructure:"MINIO_ACCESS_KEY"`
	MinioSecretKey string `mapstructure:"MINIO_SECRET_KEY"`
	MinioBucket    string `mapstructure:"MINIO_BUCKET"`
	MinioUseSSL    bool   `mapstructure:"MINIO_USE_SSL"`

	ResumeGroundingLvl string `mapstructure:"RESUME_GROUNDING_LEVEL"`
	RendercvBin        string `mapstructure:"RENDERCV_BIN"`
	ResumeMasterPath   string `mapstructure:"RESUME_MASTER_PATH"`

	LevelsFyiCSV   string `mapstructure:"LEVELS_FYI_CSV"`
	SalaryFloorUsd int    `mapstructure:"SALARY_FLOOR_USD"`

	LinkedInScrapeEnabled bool `mapstructure:"LINKEDIN_SCRAPE_ENABLED"`

	AIConcurrencyCloud int `mapstructure:"AI_CONCURRENCY_CLOUD"`
	IngestConcurrency  int `mapstructure:"INGEST_CONCURRENCY"`
	EnrichConcurrency  int `mapstructure:"ENRICH_CONCURRENCY"`

	IngestPersistChunkSize int `mapstructure:"INGEST_PERSIST_CHUNK_SIZE"`

	AITaskTimeoutMatch    time.Duration `mapstructure:"AI_TASK_TIMEOUT_MATCH"`
	AITaskTimeoutGenerate time.Duration `mapstructure:"AI_TASK_TIMEOUT_GENERATE"`
	AITaskTimeoutSalary   time.Duration `mapstructure:"AI_TASK_TIMEOUT_SALARY"`
	AITaskTimeoutGhost    time.Duration `mapstructure:"AI_TASK_TIMEOUT_GHOST"`
	AITaskTimeoutEnrich   time.Duration `mapstructure:"AI_TASK_TIMEOUT_ENRICH"`
	AITaskTimeoutIngest   time.Duration `mapstructure:"AI_TASK_TIMEOUT_INGEST"`

	ActivityHeartbeatInterval time.Duration `mapstructure:"ACTIVITY_HEARTBEAT_INTERVAL"`
	ActivityStaleAfter        time.Duration `mapstructure:"ACTIVITY_STALE_AFTER"`
	ActivitySweepInterval     time.Duration `mapstructure:"ACTIVITY_SWEEP_INTERVAL"`
	ActivityQueuedGrace       time.Duration `mapstructure:"ACTIVITY_QUEUED_GRACE"`

	LLMMaxIdleConnsPerHost int `mapstructure:"LLM_MAX_IDLE_CONNS_PER_HOST"`

	// LLM observability retention (036 FR-008, contracts C7-1). The platform
	// prunes the collector itself because automated retention is not an OSS
	// feature there.
	//
	// These are named EVAL_PRUNE_* rather than LANGFUSE_* deliberately: C2-2
	// forbids granting the application container a LANGFUSE_* variable, and
	// this is a different grant for a different reason — a collector API key
	// so the platform can delete, never a provider credential — so it is
	// separately named and separately revocable.
	EvalPruneCollectorURL string `mapstructure:"EVAL_PRUNE_COLLECTOR_URL"`
	EvalPrunePublicKey    string `mapstructure:"EVAL_PRUNE_PUBLIC_KEY"`
	EvalPruneSecretKey    string `mapstructure:"EVAL_PRUNE_SECRET_KEY"`
	EvalPruneRetentionDay int    `mapstructure:"EVAL_PRUNE_RETENTION_DAYS"`

	DBMaxConns           int           `mapstructure:"DB_MAX_CONNS"`
	DBMinConns           int           `mapstructure:"DB_MIN_CONNS"`
	DBMaxConnLifetime    time.Duration `mapstructure:"DB_MAX_CONN_LIFETIME"`
	DBMaxConnIdleTime    time.Duration `mapstructure:"DB_MAX_CONN_IDLE_TIME"`
	DBAcquireTimeout     time.Duration `mapstructure:"DB_ACQUIRE_TIMEOUT"`
	DBServerMaxConns     int           `mapstructure:"DB_SERVER_MAX_CONNS"`
	DBInteractiveReserve int           `mapstructure:"DB_INTERACTIVE_RESERVE"`
}

// Load reads configuration for binaries that do AI/inference work. It applies
// every validation LoadNonAI does, plus the AI surface requirement (K1):
// GATEWAY_URL and LITELLM_MASTER_KEY must be configured. It never checks that
// the gateway is reachable — only that it is configured (K1-2, K1-3).
func Load() (*Config, error) {
	cfg, err := load()
	if err != nil {
		return nil, err
	}
	if err := validateAISurface(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadNonAI reads configuration for binaries and tests that do no inference
// work, so must not be required to configure a gateway (K1-4).
func LoadNonAI() (*Config, error) {
	return load()
}

func load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()

	for k, val := range defaults {
		v.SetDefault(k, val)
	}
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

// validateAISurface enforces K1: the gateway must be configured before any
// AI-doing binary boots. This checks presence only, never reachability — an
// unreachable gateway fails tasks, not boots (K1-2, K1-3).
func validateAISurface(cfg *Config) error {
	if cfg.GatewayURL == "" {
		return fmt.Errorf("config: GATEWAY_URL is required")
	}
	if cfg.LiteLLMMasterKey == "" {
		return fmt.Errorf("config: LITELLM_MASTER_KEY is required")
	}
	if cfg.RabbitMQURL == "" {
		return fmt.Errorf("config: RABBITMQ_URL is required")
	}
	return nil
}
