package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}

	platform, err := buildPlatform(ctx, cfg)
	if err != nil {
		return err
	}
	defer platform.DB.Close()
	defer platform.Scraping.Close()
	defer platform.RabbitConn.Close()

	app, err := buildContexts(ctx, platform)
	if err != nil {
		return err
	}

	servers := buildServers(platform, app)
	return runServers(ctx, platform, servers, app.Scheduler)
}
