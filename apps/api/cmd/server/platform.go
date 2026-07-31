package main

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/jobsources/infrastructure/adapters"
	"github.com/job-finder/api/internal/platform/scraping"
	"github.com/job-finder/api/internal/queue"
)

// Platform holds the process-wide shared infrastructure that every feature
// composer draws on: the database pool, logger, Redis/asynq client, the
// headless-scraping service, and the djinni session shared by pointer across
// the adapter registry and the enrichment handler.
type Platform struct {
	Config         *config.Config
	DB             *db.DB
	Logger         *slog.Logger
	RedisOpt       asynq.RedisClientOpt
	AsynqClient    *asynq.Client
	AsynqInspector *asynq.Inspector
	Scraping       *scraping.Service

	// Policies is the resolved per-task-type concurrency/deadline
	// configuration (019-ai-job-throughput), looked up by policyFor.
	Policies []queue.TaskPolicy

	// Sweeper reclaims activity runs stuck "running" past their deadline
	// (019-ai-job-throughput).
	Sweeper *activity.Sweeper

	// DjinniSession is shared by pointer with every DjinniAdapter copy
	// (registry + enrichment handler); its Sources back-reference is wired once
	// jobsources.Service exists (see composeJobSources), breaking the
	// adapter<->service construction cycle.
	DjinniSession *adapters.DjinniSession
}

// buildPlatform opens the shared infrastructure. Callers own the lifecycle:
// close DB, Scraping, and AsynqClient once buildPlatform returns nil error.
func buildPlatform(ctx context.Context, cfg *config.Config) (*Platform, error) {
	// Policies come first: they size both the asynq worker pools and the
	// connection budget, so the two cannot drift apart (026-db-pool-capacity).
	policies, err := queue.PoliciesFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	poolCfg, err := poolConfigFor(cfg, policies)
	if err != nil {
		return nil, err
	}

	database, err := db.Open(ctx, cfg.DatabaseURL, db.WithPoolConfig(poolCfg))
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

	asynqInspector := asynq.NewInspector(redisOpt)
	sweeper := activity.NewSweeper(database.Queries, asynqInspector,
		cfg.ActivityStaleAfter, cfg.ActivitySweepInterval, cfg.ActivityQueuedGrace)

	return &Platform{
		Config:         cfg,
		DB:             database,
		Logger:         slog.Default(),
		RedisOpt:       redisOpt,
		AsynqClient:    asynq.NewClient(redisOpt),
		AsynqInspector: asynqInspector,
		Scraping:       scrapingSvc,
		Policies:       policies,
		Sweeper:        sweeper,
		DjinniSession:  &adapters.DjinniSession{Email: cfg.DjinniEmail, Password: cfg.DjinniPassword, Key: "djinni"},
	}, nil
}

// poolConfigFor derives and validates the connection-capacity policy, then
// states it in one log line so the effective policy is visible without reading
// configuration (026-db-pool-capacity, contracts/config.md).
func poolConfigFor(cfg *config.Config, policies []queue.TaskPolicy) (db.PoolConfig, error) {
	budget := db.BudgetFromPolicies(policies, cfg.DBInteractiveReserve, cfg.DBServerMaxConns)
	required := budget.Required()
	if err := config.ValidateDBCapacity(cfg, budget.WorkerSlots, budget.BackgroundSlots, required); err != nil {
		return db.PoolConfig{}, err
	}

	maxConns := cfg.EffectiveDBMaxConns(required)
	slog.Info("db pool configured",
		"max_conns", maxConns,
		"derived", cfg.DBMaxConns == 0,
		"workers", budget.WorkerSlots,
		"background", budget.BackgroundSlots,
		"reserve", budget.InteractiveReserve,
		"min_conns", cfg.DBMinConns,
		"lifetime", cfg.DBMaxConnLifetime,
		"idle", cfg.DBMaxConnIdleTime,
		"acquire_timeout", cfg.DBAcquireTimeout,
	)

	return db.PoolConfig{
		MaxConns:        int32(maxConns),
		MinConns:        int32(cfg.DBMinConns),
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	}, nil
}
