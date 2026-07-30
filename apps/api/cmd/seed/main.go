// Command seed populates the local development database with realistic
// fixture data. Run via: make seed
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/jobsources/infrastructure/adapters"
	"github.com/job-finder/api/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed: fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	clean := flag.Bool("clean", false, "truncate all tables before seeding (FK-safe order)")
	tables := flag.String("tables", "", "comma-separated list of tables to seed (empty = all)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx := context.Background()

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	if *clean {
		if err := seed.Clean(ctx, database.Pool); err != nil {
			return fmt.Errorf("seed: clean: %w", err)
		}
		slog.Info("seed: truncated all tables")
	}

	// Job sources are defined in code (the adapter registry); JobSource rows
	// are created lazily on first use, not seeded upfront. The fixtures below
	// (SourceRun, Subscription) FK against those rows, so materialize one per
	// adapter here via the same lazy GetByKey path the running server uses.
	registry := domain.NewRegistry(
		adapters.AdzunaAdapter{AppID: cfg.AdzunaAppID, AppKey: cfg.AdzunaAppKey, Country: cfg.AdzunaCountry},
		adapters.RemotiveAdapter{},
		adapters.ArbeitnowAdapter{},
		adapters.DjinniAdapter{},
		adapters.DouAdapter{},
		adapters.WorkUaAdapter{},
		adapters.RobotaAdapter{},
		adapters.JobSpyAdapter{URL: cfg.JobspyURL},
		adapters.JoobleAdapter{APIKey: cfg.JoobleAPIKey},
	)
	sourcesSvc := application.NewService(database.Queries, registry, cfg.ConfigEncryptionKey)
	for _, a := range registry.All() {
		if _, err := sourcesSvc.GetByKey(ctx, a.Key()); err != nil {
			return fmt.Errorf("seed: job source %s: %w", a.Key(), err)
		}
	}

	opts := seed.Options{}
	if *tables != "" {
		opts.Tables = strings.Split(*tables, ",")
	}

	if err := seed.Run(ctx, database.Pool, database.Queries, opts); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	return nil
}
