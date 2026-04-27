// Command server is the single-binary entrypoint: it runs the HTTP API,
// the asynq worker, and the ingestion scheduler as goroutines in one
// process, mirroring the NestJS app's single-process model.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/job-finder/api-go/internal/config"
	"github.com/job-finder/api-go/internal/db"
	"github.com/job-finder/api-go/internal/httpapi"
	"github.com/job-finder/api-go/internal/jobsources"
	"github.com/job-finder/api-go/internal/jobsources/adapters"
	"github.com/job-finder/api-go/internal/scraping"
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

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	scrapingSvc := scraping.New()
	defer scrapingSvc.Close()

	registry := jobsources.NewRegistry(
		adapters.AdzunaAdapter{},
		adapters.RemotiveAdapter{},
		adapters.ArbeitnowAdapter{},
		adapters.DjinniAdapter{Scraping: scrapingSvc},
		adapters.DouAdapter{Scraping: scrapingSvc},
		adapters.JobSpyAdapter{},
	)
	sourcesSvc := jobsources.NewService(database.Queries, registry, cfg.ConfigEncryptionKey)
	if err := sourcesSvc.Seed(ctx); err != nil {
		return err
	}
	sourcesHandler := &httpapi.SourcesHandler{Sources: sourcesSvc}

	router := httpapi.NewRouter(sourcesHandler.Mount)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("API listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
