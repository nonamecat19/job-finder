package main

import (
	"context"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"github.com/nonamecat19/job-scraper/model"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/scraping"
	"github.com/nonamecat19/job-scraper/adapters/djinni"
	"github.com/nonamecat19/job-scraper/adapters/jobleads"
	"github.com/nonamecat19/job-scraper/ports"
)

type Platform struct {
	Config      *config.Config
	DB          *db.DB
	Logger      *slog.Logger
	RedisClient redis.UniversalClient
	Scraping    *scraping.HTTPScraper

	// RabbitConn is the connection topology is declared on and the publisher
	// publishes over. Consumers (servers.go) dial their own connections —
	// each Consumer reconnects independently on failure (M3-4).
	RabbitConn  *amqp.Connection
	Publisher   *events.Publisher
	Enqueuer    queue.Enqueuer
	RabbitAdmin *events.Admin
	Metrics     *events.Metrics

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

	redisClient, err := queue.NewRedisClient(cfg.RedisURL)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		return nil, err
	}

	rabbitConn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		return nil, fmt.Errorf("platform: dial rabbitmq: %w", err)
	}
	rabbitCh, err := rabbitConn.Channel()
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		_ = rabbitConn.Close()
		return nil, fmt.Errorf("platform: open rabbitmq channel: %w", err)
	}
	if err := events.DeclareTopology(rabbitCh); err != nil {
		database.Close()
		scrapingSvc.Close()
		_ = rabbitConn.Close()
		return nil, err
	}
	publisher, err := events.NewPublisher(rabbitCh, 0)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		_ = rabbitConn.Close()
		return nil, err
	}
	var enqueuer queue.Enqueuer = &events.PublishEnqueuer{Publisher: publisher}

	admin, err := events.NewAdmin(cfg.RabbitMQURL)
	if err != nil {
		database.Close()
		scrapingSvc.Close()
		_ = rabbitConn.Close()
		return nil, err
	}

	sweeper := activity.NewSweeper(database.Queries,
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
		Config:      cfg,
		DB:          database,
		Logger:      slog.Default(),
		RedisClient: redisClient,
		Scraping:    scrapingSvc,

		RabbitConn:  rabbitConn,
		Publisher:   publisher,
		Enqueuer:    enqueuer,
		RabbitAdmin: admin,
		Metrics:     events.NewMetrics(),

		Policies: policies,
		Sweeper:  sweeper,

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
