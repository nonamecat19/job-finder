package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	jsadapter "github.com/nonamecat19/jobscraper/adapter"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/seed"
	"github.com/nonamecat19/jobscraper/adapters/adzuna"
	"github.com/nonamecat19/jobscraper/adapters/arbeitnow"
	"github.com/nonamecat19/jobscraper/adapters/djinni"
	"github.com/nonamecat19/jobscraper/adapters/dou"
	"github.com/nonamecat19/jobscraper/adapters/jobspy"
	"github.com/nonamecat19/jobscraper/adapters/jooble"
	"github.com/nonamecat19/jobscraper/adapters/manual"
	"github.com/nonamecat19/jobscraper/adapters/remotive"
	"github.com/nonamecat19/jobscraper/adapters/robota"
	"github.com/nonamecat19/jobscraper/adapters/workua"
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

	// Seeding only needs the source keys to exist as rows, so the sources are
	// built with nothing behind them: an empty scraper is enough for a value
	// that is never asked to crawl.
	registry, err := jsadapter.NewRegistry(
		adzuna.New(cfg.AdzunaAppID, cfg.AdzunaAppKey, cfg.AdzunaCountry),
		remotive.New(),
		arbeitnow.New(),
		djinni.Source{},
		dou.Source{},
		workua.Source{},
		robota.New(),
		jobspy.New(cfg.JobspyURL),
		jooble.New(cfg.JoobleAPIKey),
		manual.New(),
	)
	if err != nil {
		return err
	}
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
