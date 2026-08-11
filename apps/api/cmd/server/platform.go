package main

import (
	"context"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/jobscraper/adapters"
	"github.com/job-finder/jobscraper/scraping"
)

type Platform struct {
	Config         *config.Config
	DB             *db.DB
	Logger         *slog.Logger
	RedisOpt       asynq.RedisClientOpt
	AsynqClient    *asynq.Client
	AsynqInspector *asynq.Inspector
	Scraping       *scraping.HTTPScraper

	Policies []queue.TaskPolicy

	Sweeper *activity.Sweeper

	DjinniSession *adapters.DjinniSession
}

func buildPlatform(ctx context.Context, cfg *config.Config) (*Platform, error) {
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
