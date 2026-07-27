package main

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/retrieval"
	"github.com/job-finder/api/internal/scraping"
)

// Platform holds the process-wide shared infrastructure that every feature
// composer draws on: the database pool, logger, Redis/asynq client, the
// headless-scraping service, and the jobleads session shared by pointer across
// the adapter registry and the enrichment handler.
type Platform struct {
	Config         *config.Config
	DB             *db.DB
	Logger         *slog.Logger
	RedisOpt       asynq.RedisClientOpt
	RedisClient    redis.UniversalClient
	AsynqClient    *asynq.Client
	AsynqInspector *asynq.Inspector
	Scraping       *scraping.Service
	Retrieval      retrieval.Service

	// MinioReady is a lightweight client used only to probe MinIO connectivity
	// for the readiness endpoint (see internal/httpapi/health.go). It is nil
	// when cfg.MinioEndpoint is unset, matching the "MinIO disabled" convention
	// used by internal/storage.NewMinioStore.
	MinioReady *minio.Client

	// JobLeadsSession is the same pattern as DjinniSession, for the
	// login-gated JobLeads source.
	JobLeadsSession *adapters.JobLeadsSession
}

// buildPlatform opens the shared infrastructure. Callers own the lifecycle:
// close DB, Scraping, and AsynqClient once buildPlatform returns nil error.
func buildPlatform(ctx context.Context, cfg *config.Config) (*Platform, error) {
	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// Initialise the browser identity and retrieval state store.
	identityVersion := cfg.BrowserIdentityVersion
	if identityVersion == "" {
		identityVersion = "chrome126"
	}
	identity, err := retrieval.NewBrowserIdentity(identityVersion)
	if err != nil {
		database.Close()
		return nil, err
	}
	stateStore := retrieval.NewStateStore(database.Queries, cfg.ConfigEncryptionKey)
	retrieval.ConfigureDefaultTransport(stateStore, nil)
	retSvc, err := retrieval.NewServiceImpl(identity, stateStore, cfg)
	if err != nil {
		database.Close()
		return nil, err
	}

	scrapingSvc := scraping.New(retSvc)

	redisOpt, err := queue.RedisOpt(cfg.RedisURL)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		return nil, err
	}

	var minioReady *minio.Client
	if cfg.MinioEndpoint != "" {
		minioReady, err = minio.New(cfg.MinioEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
			Secure: cfg.MinioUseSSL,
		})
		if err != nil {
			database.Close()
			scrapingSvc.Close()
			return nil, err
		}
	}

	return &Platform{
		Config:          cfg,
		DB:              database,
		Logger:          slog.Default(),
		RedisOpt:        redisOpt,
		RedisClient:     redisOpt.MakeRedisClient().(redis.UniversalClient),
		AsynqClient:     asynq.NewClient(redisOpt),
		AsynqInspector:  asynq.NewInspector(redisOpt),
		Scraping:        scrapingSvc,
		Retrieval:       retSvc,
		MinioReady:      minioReady,
		JobLeadsSession: &adapters.JobLeadsSession{Email: cfg.JobLeadsEmail, Password: cfg.JobLeadsPassword, Key: "jobleads"},
	}, nil
}
