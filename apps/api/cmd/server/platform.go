package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/nonamecat19/jobscraper/model"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/scraping"
	"github.com/nonamecat19/jobscraper/adapters/djinni"
	"github.com/nonamecat19/jobscraper/adapters/jobleads"
	"github.com/nonamecat19/jobscraper/ports"
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

	// SourceConfig is where the credentialed sessions read their credentials
	// and persist the cookie they obtain. It is bound to the job-sources
	// service once that exists — see composeJobSources.
	SourceConfig *lateSourceConfig

	DjinniSession   ports.SessionProvider
	JobLeadsSession ports.SessionProvider
}

// lateSourceConfig is a ports.SourceConfigStore that gets its real store after
// construction, breaking the cycle between the sessions (which the sources
// need), the sources (which the registry needs) and the job-sources service
// (which needs the registry and *is* the store).
//
// Calls before binding fail rather than silently reading nothing: nothing
// should reach a session that early, and a clear error beats a mystery
// logged-out crawl.
type lateSourceConfig struct{ inner ports.SourceConfigStore }

var _ ports.SourceConfigStore = (*lateSourceConfig)(nil)

func (l *lateSourceConfig) Bind(store ports.SourceConfigStore) { l.inner = store }

func (l *lateSourceConfig) Config(ctx context.Context, key string) (map[string]any, error) {
	if l.inner == nil {
		return nil, fmt.Errorf("source config store for %q is not bound yet", key)
	}
	return l.inner.Config(ctx, key)
}

func (l *lateSourceConfig) Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*model.JobSourceDto, error) {
	if l.inner == nil {
		return nil, fmt.Errorf("source config store for %q is not bound yet", key)
	}
	return l.inner.Update(ctx, key, enabled, configPatch)
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

	sourceConfig := &lateSourceConfig{}
	djinniSession, err := djinni.NewSession(sourceConfig, cfg.DjinniEmail, cfg.DjinniPassword)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		return nil, err
	}
	jobLeadsSession, err := jobleads.NewSession(sourceConfig, cfg.JobLeadsEmail, cfg.JobLeadsPassword)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		return nil, err
	}

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

		SourceConfig:    sourceConfig,
		DjinniSession:   djinniSession,
		JobLeadsSession: jobLeadsSession,
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
