package main

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/scraping"
)

// Platform holds the process-wide shared infrastructure that every feature
// composer draws on: the database pool, logger, Redis/asynq client, the
// headless-scraping service, and the djinni session shared by pointer across
// the adapter registry and the enrichment handler.
type Platform struct {
	Config      *config.Config
	DB          *db.DB
	Logger      *slog.Logger
	RedisOpt    asynq.RedisClientOpt
	AsynqClient *asynq.Client
	Scraping    *scraping.Service

	// DjinniSession is shared by pointer with every DjinniAdapter copy
	// (registry + enrichment handler); its Sources back-reference is wired once
	// jobsources.Service exists (see composeJobSources), breaking the
	// adapter<->service construction cycle.
	DjinniSession *adapters.DjinniSession

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

	scrapingSvc := scraping.New()

	redisOpt, err := queue.RedisOpt(cfg.RedisURL)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		return nil, err
	}

	return &Platform{
		Config:          cfg,
		DB:              database,
		Logger:          slog.Default(),
		RedisOpt:        redisOpt,
		AsynqClient:     asynq.NewClient(redisOpt),
		Scraping:        scrapingSvc,
		DjinniSession:   &adapters.DjinniSession{Email: cfg.DjinniEmail, Password: cfg.DjinniPassword, Key: "djinni"},
		JobLeadsSession: &adapters.JobLeadsSession{Email: cfg.JobLeadsEmail, Password: cfg.JobLeadsPassword, Key: "jobleads"},
	}, nil
}
