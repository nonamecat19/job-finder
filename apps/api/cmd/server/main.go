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

	"github.com/job-finder/api/internal/applications"
	"github.com/job-finder/api/internal/companyintel"
	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/jobs"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/keyword"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/postage"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/recruiter"
	"github.com/job-finder/api/internal/salary"
	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/storage"
	"github.com/job-finder/api/internal/subscriptions"
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

	// djinniSession is shared by pointer with every DjinniAdapter copy (registry
	// + enrichment handler); its Sources back-reference is wired once sourcesSvc
	// exists (below), breaking the adapter<->service construction cycle.
	djinniSession := &adapters.DjinniSession{Email: cfg.DjinniEmail, Password: cfg.DjinniPassword, Key: "djinni"}
	djinniAdapter := adapters.DjinniAdapter{Scraping: scrapingSvc, Session: djinniSession}
	douAdapter := adapters.DouAdapter{Scraping: scrapingSvc}
	workuaAdapter := adapters.WorkUaAdapter{Scraping: scrapingSvc}
	registry := jobsources.NewRegistry(
		adapters.AdzunaAdapter{},
		adapters.RemotiveAdapter{},
		adapters.ArbeitnowAdapter{},
		djinniAdapter,
		douAdapter,
		workuaAdapter,
		adapters.RobotaAdapter{},
		adapters.JobSpyAdapter{},
		adapters.JoobleAdapter{},
	)
	sourcesSvc := jobsources.NewService(database.Queries, registry, cfg.ConfigEncryptionKey)
	djinniSession.Sources = sourcesSvc

	redisOpt, err := queue.RedisOpt(cfg.RedisURL)
	if err != nil {
		return err
	}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	ingestionSvc := ingestion.NewService(database.Queries, registry, sourcesSvc, asynqClient)
	ingestionHandler := ingestion.NewHandler(database.Queries, registry, sourcesSvc, asynqClient)
	scheduler := ingestion.NewScheduler(database.Queries, ingestionSvc)
	sourcesHandler := &httpapi.SourcesHandler{Sources: sourcesSvc, Ingestion: ingestionSvc}
	searchesHandler := &httpapi.SearchesHandler{Ingestion: ingestionSvc}

	llmProvider, err := llm.New(cfg)
	if err != nil {
		return err
	}
	profileSvc := profile.NewService(database.Queries, llmProvider, cfg.EmbedModel, cfg.RendercvBin)
	matchingSvc := matching.NewService(database.Queries, profileSvc, llmProvider, cfg.MatchSimilarityThreshold, cfg.ModelOr(cfg.LLMModelMatch))
	notifierSvc := notifier.NewService(database.Queries,
		notifier.WithMatchThreshold(cfg.MatchNotifyScoreThreshold),
		notifier.WithRateLimitCap(cfg.MatchNotifyRateLimit),
	)
	matchingHandler := matching.NewHandler(matchingSvc, notifierSvc)

	// MinIO object storage: when configured, every rendered resume/cover-letter
	// file is uploaded here in addition to the local DocumentsDir.
	var blobStore storage.Blobstore
	if cfg.MinioEndpoint != "" {
		ms, err := storage.NewMinioStore(ctx, storage.Config{
			Endpoint:  cfg.MinioEndpoint,
			AccessKey: cfg.MinioAccessKey,
			SecretKey: cfg.MinioSecretKey,
			Bucket:    cfg.MinioBucket,
			UseSSL:    cfg.MinioUseSSL,
		})
		if err != nil {
			return err
		}
		blobStore = ms
		slog.Info("MinIO object storage enabled", "endpoint", cfg.MinioEndpoint, "bucket", cfg.MinioBucket)
	}

	htmlRenderer, err := generation.NewHtmlPdfRenderer(scrapingSvc, cfg.DocumentsDir)
	if err != nil {
		return err
	}
	htmlRenderer.Store = blobStore
	rendercvRenderer := generation.NewRenderCvRenderer(cfg.DocumentsDir, cfg.RendercvBin)
	rendercvRenderer.Store = blobStore
	generationSvc := generation.NewService(database.Queries, profileSvc, htmlRenderer, rendercvRenderer, llmProvider, cfg.ModelOr(cfg.LLMModelGeneration), cfg.ResumeMasterPath, cfg.ResumeGroundingLvl)
	generationHandler := generation.NewHandler(generationSvc)
	documentsHandler := &httpapi.DocumentsHandler{Generation: generationSvc}

	profilesHandler := &httpapi.ProfilesHandler{Profiles: profileSvc}
	jobsSvc := jobs.NewService(database.Queries, asynqClient, cfg.SalaryFloorUsd)
	jobsHandler := &httpapi.JobsHandler{Jobs: jobsSvc, Generation: generationSvc}
	// database (not database.Queries) is passed as the TxRunner so a status
	// change and its "ApplicationOutcome" event commit atomically.
	applicationsSvc := applications.NewService(database.Queries, database)
	applicationsHandler := &httpapi.ApplicationsHandler{Applications: applicationsSvc}
	subsSvc := subscriptions.NewService(database.Queries, sourcesSvc)
	subsHandler := &httpapi.SubscriptionsHandler{Subs: subsSvc, Ingestion: ingestionSvc}

	enrichDelay := time.Duration(cfg.DjinniDetailDelayMs) * time.Millisecond
	enrichDelays := map[string]time.Duration{
		"workua": time.Duration(cfg.WorkUaDetailDelayMs) * time.Millisecond,
	}
	enrichHandler := enrichment.NewHandler(database.Queries, sourcesSvc, djinniAdapter, douAdapter, workuaAdapter, asynqClient, enrichDelay, enrichDelays)
	sourcesHandler.Enrichment = enrichHandler

	levelsFyiLoader := salary.NewLevelsFyiLoader(database.Queries)
	salaryService := salary.NewService(database.Queries, llmProvider, levelsFyiLoader, cfg.ModelOr(""))
	salaryHandler := salary.NewHandler(salaryService)

	if cfg.LevelsFyiCSV != "" {
		if _, err := levelsFyiLoader.LoadCSV(ctx, cfg.LevelsFyiCSV); err != nil {
			slog.Warn("salary: levels.fyi CSV load failed", "path", cfg.LevelsFyiCSV, "error", err)
		}
	} else {
		slog.Warn("salary: LEVELS_FYI_CSV not set — levels.fyi source disabled")
	}

	activityHandler := httpapi.NewActivityHandler(database.Queries)

	// Keyword-diff endpoint (008-6): reads the KeywordDiff cache (008-4) and
	// attaches advisory rephrase suggestions (008-5). Each rephrase is a live
	// LLM call per missing-required term, so it must not run inline on the GET.
	// keyword.CachedRephraser fronts the Suggester with an async, TTL'd cache:
	// the first request returns empty suggestions and computes them in the
	// background; later requests return the cached result. Profile bullets are
	// the grounding source, so the suggester only reframes existing experience.
	rephraseModel := keyword.NewProviderRephraseModel(llmProvider, cfg.ModelOr(cfg.LLMModelRephrase))
	cachedRephraser := keyword.NewCachedRephraser(
		keyword.NewSuggester(rephraseModel),
		time.Duration(cfg.KeywordRephraseCacheTTLSec)*time.Second,
	)
	diffService := keyword.NewDiffService(database.Queries).WithRephraser(cachedRephraser, profileSvc)
	keywordHandler := &httpapi.KeywordHandler{Diff: diffService}

	postageSvc := postage.NewService(database.Queries)
	postageHandler := &httpapi.PostAgeHandler{PostAge: postageSvc}

	notificationSvc := notifier.NewNotificationService(database.Queries, database.Queries)
	notificationHandler := &httpapi.NotificationHandler{Provider: notificationSvc}

	companyIntelRegistry := companyintel.NewRegistry(
		companyintel.CrunchbaseScraper{Scraping: scrapingSvc},
		companyintel.LayoffsScraper{Scraping: scrapingSvc},
		companyintel.GlassdoorScraper{Scraping: scrapingSvc},
		companyintel.HeadcountScraper{Scraping: scrapingSvc},
		companyintel.TechStackScraper{Scraping: scrapingSvc},
	)
	companyIntelSvc := companyintel.NewService(database.Queries, companyIntelRegistry, 2*time.Second)
	companiesHandler := &httpapi.CompaniesHandler{CompanyIntel: companyIntelSvc}

	// Recruiter/hiring-manager resolution (007): posting-text always runs;
	// the company-page source reuses scrapingSvc the same way companyintel's
	// HeadcountScraper does, and LinkedIn only runs when the operator has
	// opted in via LINKEDIN_SCRAPE_ENABLED.
	recruiterSvc := recruiter.NewService(database.Queries, llmProvider, cfg.ModelOr(""), scrapingSvc, cfg.LinkedInScrapeEnabled)
	contactsHandler := &httpapi.ContactsHandler{Recruiter: recruiterSvc}

	router := httpapi.NewRouter(
		sourcesHandler.Mount, searchesHandler.Mount, documentsHandler.Mount,
		profilesHandler.Mount, jobsHandler.Mount, applicationsHandler.Mount,
		subsHandler.Mount, activityHandler.Mount, keywordHandler.Mount,
		postageHandler.Mount, notificationHandler.Mount, companiesHandler.Mount,
		contactsHandler.Mount,
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

	// Concurrency 1: enrichment hits an authenticated personal djinni page
	// per job, serialized + delayed (DJINNI_DETAIL_DELAY_MS) to stay polite.
	enrichMux := asynq.NewServeMux()
	enrichMux.HandleFunc(queue.TypeEnrich, enrichHandler.ProcessTask)
	enrichSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{queue.QueueEnrich: 1},
	})

	salaryMux := asynq.NewServeMux()
	salaryMux.HandleFunc(queue.TypeSalaryInfer, salaryHandler.ProcessTask)
	salaryAsynqSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{queue.QueueSalaryInfer: 1},
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
	go func() {
		if err := enrichSrv.Run(enrichMux); err != nil {
			slog.Error("asynq enrich worker error", "error", err)
		}
	}()
	go func() {
		if err := salaryAsynqSrv.Run(salaryMux); err != nil {
			slog.Error("asynq salary worker error", "error", err)
		}
	}()

	go scheduler.Run(ctx)

	<-ctx.Done()
	slog.Info("shutting down")
	ingestSrv.Shutdown()
	matchSrv.Shutdown()
	generateSrv.Shutdown()
	enrichSrv.Shutdown()
	salaryAsynqSrv.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
