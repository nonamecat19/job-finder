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

	"github.com/hibiken/asynq"

	"github.com/job-finder/api-go/internal/applications"
	"github.com/job-finder/api-go/internal/config"
	"github.com/job-finder/api-go/internal/db"
	"github.com/job-finder/api-go/internal/generation"
	"github.com/job-finder/api-go/internal/httpapi"
	"github.com/job-finder/api-go/internal/ingestion"
	"github.com/job-finder/api-go/internal/jobs"
	"github.com/job-finder/api-go/internal/jobsources"
	"github.com/job-finder/api-go/internal/jobsources/adapters"
	"github.com/job-finder/api-go/internal/llm"
	"github.com/job-finder/api-go/internal/matching"
	"github.com/job-finder/api-go/internal/profile"
	"github.com/job-finder/api-go/internal/queue"
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

	redisOpt, err := queue.RedisOpt(cfg.RedisURL)
	if err != nil {
		return err
	}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	ingestionSvc := ingestion.NewService(database.Queries, registry, asynqClient)
	ingestionHandler := ingestion.NewHandler(database.Queries, registry, sourcesSvc, asynqClient)
	scheduler := ingestion.NewScheduler(database.Queries, ingestionSvc)
	searchesHandler := &httpapi.SearchesHandler{Ingestion: ingestionSvc}

	llmProvider, err := llm.New(cfg)
	if err != nil {
		return err
	}
	profileSvc := profile.NewService(database.Queries, llmProvider, cfg.EmbedModel)
	matchingSvc := matching.NewService(database.Queries, profileSvc, llmProvider, cfg.MatchSimilarityThreshold)
	matchingHandler := matching.NewHandler(matchingSvc)

	htmlRenderer, err := generation.NewHtmlPdfRenderer(scrapingSvc, cfg.DocumentsDir)
	if err != nil {
		return err
	}
	rendercvRenderer := generation.NewRenderCvRenderer(cfg.DocumentsDir, cfg.RendercvBin)
	generationSvc := generation.NewService(database.Queries, profileSvc, htmlRenderer, rendercvRenderer, llmProvider, cfg.ResumeMasterPath, cfg.ResumeGroundingLvl)
	generationHandler := generation.NewHandler(generationSvc)
	documentsHandler := &httpapi.DocumentsHandler{Generation: generationSvc}

	profilesHandler := &httpapi.ProfilesHandler{Profiles: profileSvc}
	jobsSvc := jobs.NewService(database.Queries, asynqClient)
	jobsHandler := &httpapi.JobsHandler{Jobs: jobsSvc, Generation: generationSvc}
	applicationsSvc := applications.NewService(database.Queries)
	applicationsHandler := &httpapi.ApplicationsHandler{Applications: applicationsSvc}

	router := httpapi.NewRouter(
		sourcesHandler.Mount, searchesHandler.Mount, documentsHandler.Mount,
		profilesHandler.Mount, jobsHandler.Mount, applicationsHandler.Mount,
	)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Each task type gets its own asynq.Server (own worker pool + own queue),
	// so its Concurrency is a hard per-queue cap rather than a weight shared
	// across task types — matching the BullMQ setup's separate queues:
	// ingest processor.concurrency=2 (ingestion.processor.ts), match and
	// generate concurrency=1 each ("local LLM handles one request at a time
	// comfortably" — matching.processor.ts, generation.processor.ts). A
	// single asynq.Server's Queues map only sets priority *weights* within
	// one shared pool, which can't reproduce a hard cap of 1.
	ingestMux := asynq.NewServeMux()
	ingestMux.HandleFunc(queue.TypeIngest, ingestionHandler.ProcessTask)
	ingestSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 2,
		Queues:      map[string]int{queue.QueueIngest: 1},
	})

	matchMux := asynq.NewServeMux()
	matchMux.HandleFunc(queue.TypeMatch, matchingHandler.ProcessTask)
	matchSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{queue.QueueMatch: 1},
	})

	generateMux := asynq.NewServeMux()
	generateMux.HandleFunc(queue.TypeGenerate, generationHandler.ProcessTask)
	generateSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{queue.QueueGenerate: 1},
	})

	go func() {
		slog.Info("API listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()

	go func() {
		if err := ingestSrv.Run(ingestMux); err != nil {
			slog.Error("asynq ingest worker error", "error", err)
		}
	}()
	go func() {
		if err := matchSrv.Run(matchMux); err != nil {
			slog.Error("asynq match worker error", "error", err)
		}
	}()
	go func() {
		if err := generateSrv.Run(generateMux); err != nil {
			slog.Error("asynq generate worker error", "error", err)
		}
	}()

	go scheduler.Run(ctx)

	<-ctx.Done()
	slog.Info("shutting down")
	ingestSrv.Shutdown()
	matchSrv.Shutdown()
	generateSrv.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
