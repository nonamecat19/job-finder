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

	OllamaURL  string `mapstructure:"OLLAMA_URL"`
	OllamaKey  string `mapstructure:"OLLAMA_KEY"`
	LLMModel   string `mapstructure:"LLM_MODEL"`
	EmbedModel string `mapstructure:"EMBED_MODEL"`

	LLMModelMatch      string `mapstructure:"LLM_MODEL_MATCH"`
	LLMModelGeneration string `mapstructure:"LLM_MODEL_GENERATION"`
	LLMModelRephrase   string `mapstructure:"LLM_MODEL_REPHRASE"`
	LLMModelGhost      string `mapstructure:"LLM_MODEL_GHOST"`

	LLMModelGenerationAnalyze string `mapstructure:"LLM_MODEL_GENERATION_ANALYZE"`
	LLMModelGenerationSelect  string `mapstructure:"LLM_MODEL_GENERATION_SELECT"`
	LLMModelGenerationPremium string `mapstructure:"LLM_MODEL_GENERATION_PREMIUM"`
	LLMModelGenerationSummary string `mapstructure:"LLM_MODEL_GENERATION_SUMMARY"`

	GatewayURL       string `mapstructure:"GATEWAY_URL"`
	LiteLLMMasterKey string `mapstructure:"LITELLM_MASTER_KEY"`

	KeywordRephraseCacheTTLSec int `mapstructure:"KEYWORD_REPHRASE_CACHE_TTL_SEC"`

	EmbedURL string `mapstructure:"EMBED_URL"`

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
	AIConcurrencyLocal int `mapstructure:"AI_CONCURRENCY_LOCAL"`
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

	OllamaKeepAlive        string `mapstructure:"OLLAMA_KEEP_ALIVE"`
	LLMMaxIdleConnsPerHost int    `mapstructure:"LLM_MAX_IDLE_CONNS_PER_HOST"`

	DBMaxConns           int           `mapstructure:"DB_MAX_CONNS"`
	DBMinConns           int           `mapstructure:"DB_MIN_CONNS"`
	DBMaxConnLifetime    time.Duration `mapstructure:"DB_MAX_CONN_LIFETIME"`
	DBMaxConnIdleTime    time.Duration `mapstructure:"DB_MAX_CONN_IDLE_TIME"`
	DBAcquireTimeout     time.Duration `mapstructure:"DB_ACQUIRE_TIMEOUT"`
	DBServerMaxConns     int           `mapstructure:"DB_SERVER_MAX_CONNS"`
	DBInteractiveReserve int           `mapstructure:"DB_INTERACTIVE_RESERVE"`
}

func (c *Config) ModelOr(m string) string {
	if m == "" {
		return c.LLMModel
	}
	return m
}

// GenerationModelOr resolves a per-stage local model, falling back to the
// generation-wide model and then to LLM_MODEL. It exists so pinning a stage is
// optional: an operator who sets only LLM_MODEL_GENERATION still gets that
// model for every stage.
func (c *Config) GenerationModelOr(m string) string {
	if m == "" {
		m = c.LLMModelGeneration
	}
	return c.ModelOr(m)
}

func Load() (*Config, error) {
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
