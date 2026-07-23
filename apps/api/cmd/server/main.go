// Command server is the single-binary entrypoint: it runs the HTTP API,
// the asynq worker, and the ingestion scheduler as goroutines in one
// process, mirroring the NestJS app's single-process model.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/job-finder/api/internal/autogen"
	"github.com/job-finder/api/internal/coach"
	"github.com/job-finder/api/internal/companyintel"
	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/extauth"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/jobs"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/keyword"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/llmsettings"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/outreach"
	"github.com/job-finder/api/internal/postage"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/recruiter"
	"github.com/job-finder/api/internal/referral"
	"github.com/job-finder/api/internal/salary"
	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/storage"
	"github.com/job-finder/api/internal/subscriptions"
)

// extJWTSigningSecret decodes a configured 32-byte hex EXT_JWT_SECRET, or
// generates a random 32-byte secret when unset (dev/first-run convenience —
// see the EXT_JWT_SECRET comment in .env.example and config.go).
func extJWTSigningSecret(hexKey string) ([]byte, error) {
	if hexKey == "" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		slog.Warn("EXT_JWT_SECRET not set — using an ephemeral random secret; " +
			"extension sessions will not survive a server restart")
		return secret, nil
	}
	secret, err := hex.DecodeString(hexKey)
	if err != nil || len(secret) != 32 {
		return nil, errors.New("EXT_JWT_SECRET must be a 32-byte hex string (openssl rand -hex 32)")
	}
	return secret, nil
}

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

	// Cerebras free-tier model toggle (001-cerebras-model-toggle): Ollama is
	// always built; Cerebras is nil when CEREBRAS_API_KEY is unset. Each named
	// chat task gets its own llm.Router sharing one settings snapshot, so a
	// dashboard change (llmSettingsSvc.Update) takes effect for newly started
	// tasks without a restart (FR-005). Embeddings always use ollamaProvider
	// directly or via Router.Embed, which is pinned to Ollama regardless of
	// the task's chat provider (FR-006) — Cerebras has no embeddings endpoint.
	ollamaProvider, cerebrasProvider, err := llm.NewProviders(cfg)
	if err != nil {
		return err
	}
	var cerebrasIface llm.Provider
	if cerebrasProvider != nil {
		cerebrasIface = cerebrasProvider
	}
	llmSettingsSvc, err := llmsettings.NewService(ctx, database.Queries, cfg.CerebrasAPIKey != "")
	if err != nil {
		return err
	}
	llmHolder := llmSettingsSvc.Holder()
	matchRouter := llm.NewRouter("match", llmHolder, ollamaProvider, cerebrasIface)
	generationRouter := llm.NewRouter("generation", llmHolder, ollamaProvider, cerebrasIface)
	rephraseRouter := llm.NewRouter("rephrase", llmHolder, ollamaProvider, cerebrasIface)
	ghostRouter := llm.NewRouter("ghost", llmHolder, ollamaProvider, cerebrasIface)
	defaultRouter := llm.NewRouter("default", llmHolder, ollamaProvider, cerebrasIface)
	llmSettingsHandler := &httpapi.LlmSettingsHandler{Settings: llmSettingsSvc}

	// profileSvc only ever calls Embed (never chat), so it talks to Ollama
	// directly rather than through a task Router.
	profileSvc := profile.NewService(database.Queries, ollamaProvider, cfg.EmbedModel, cfg.RendercvBin)
	matchingSvc := matching.NewService(database.Queries, profileSvc, matchRouter, cfg.MatchSimilarityThreshold, "")
	notifierSvc := notifier.NewService(database.Queries,
		notifier.WithMatchThreshold(cfg.MatchNotifyScoreThreshold),
		notifier.WithRateLimitCap(cfg.MatchNotifyRateLimit),
	)
	// jobsSvc is needed early: matchingHandler auto-enqueues a resume via it
	// when a job's score crosses the autogen threshold (see below).
	jobsSvc := jobs.NewService(database.Queries, asynqClient, cfg.SalaryFloorUsd)
	autogenSvc, err := autogen.NewService(ctx, database.Queries)
	if err != nil {
		return err
	}
	autoGenerateHandler := &httpapi.AutoGenerateHandler{Settings: autogenSvc}
	matchingHandler := matching.NewHandler(matchingSvc, notifierSvc, autogenSvc, jobsSvc)

	// Ghost-job detector (005): scored at ingestion (async) and on manual
	// refresh only — never on a schedule (FR-014). Kept separate from
	// matching/fit scoring end-to-end (FR-008).
	ghostSvc := ghostjob.NewService(database.Queries, ghostRouter, "")
	ghostHandler := ghostjob.NewHandler(ghostSvc, database.Queries)
	ghostJobHandler := &httpapi.GhostJobHandler{Ghost: ghostSvc}

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
	generationSvc := generation.NewService(database.Queries, profileSvc, htmlRenderer, rendercvRenderer, generationRouter, "", cfg.ResumeMasterPath, cfg.ResumeGroundingLvl)
	generationHandler := generation.NewHandler(generationSvc)
	documentsHandler := &httpapi.DocumentsHandler{Generation: generationSvc}

	profilesHandler := &httpapi.ProfilesHandler{Profiles: profileSvc}
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
	salaryService := salary.NewService(database.Queries, defaultRouter, levelsFyiLoader, "")
	salaryHandler := salary.NewHandler(salaryService, database.Queries)

	if cfg.LevelsFyiCSV != "" {
		if _, err := levelsFyiLoader.LoadCSV(ctx, cfg.LevelsFyiCSV); err != nil {
			slog.Warn("salary: levels.fyi CSV load failed", "path", cfg.LevelsFyiCSV, "error", err)
		}
	} else {
		slog.Warn("salary: LEVELS_FYI_CSV not set — levels.fyi source disabled")
	}

	activityHandler := httpapi.NewActivityHandler(database.Queries, asynqClient)

	// Keyword-diff endpoint (008-6): reads the KeywordDiff cache (008-4) and
	// attaches advisory rephrase suggestions (008-5). Each rephrase is a live
	// LLM call per missing-required term, so it must not run inline on the GET.
	// keyword.CachedRephraser fronts the Suggester with an async, TTL'd cache:
	// the first request returns empty suggestions and computes them in the
	// background; later requests return the cached result. Profile bullets are
	// the grounding source, so the suggester only reframes existing experience.
	rephraseModel := keyword.NewProviderRephraseModel(rephraseRouter, "")
	cachedRephraser := keyword.NewCachedRephraser(
		keyword.NewSuggester(rephraseModel),
		time.Duration(cfg.KeywordRephraseCacheTTLSec)*time.Second,
	)
	diffService := keyword.NewDiffService(database.Queries).WithRephraser(cachedRephraser, profileSvc)
	keywordHandler := &httpapi.KeywordHandler{Diff: diffService}

	// Fit-gap coach (009-wiring): reads the 008 keyword diff + the profile's
	// grounding entries and produces per-gap adjacent evidence with a
	// grounded rephrase. Reuses rephraseModel (the same LLM adapter as the
	// keyword-diff suggester above) since both need the identical
	// truthful-reframing port. ProfileEntries closes over profileSvc rather
	// than coach importing internal/profile, keeping that dependency edge
	// one-directional.
	coachSvc := coach.NewService(rephraseModel)
	coachAssessSvc := coach.NewAssessmentService(coachSvc, database.Queries, func(ctx context.Context) ([]coach.ProfileEntry, error) {
		entries, err := profileSvc.ProfileEntries(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]coach.ProfileEntry, len(entries))
		for i, e := range entries {
			out[i] = coach.ProfileEntry{SourceLabel: e.SourceLabel, Bullet: e.Bullet}
		}
		return out, nil
	})
	coachHandler := &httpapi.CoachHandler{Coach: coachAssessSvc}

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

	// Browser-extension auth (014-autofill-extension). extJWTSecret must be
	// a random byte string; the hex config value is decoded when present, or
	// a fresh one is generated per process start when absent. The generated
	// fallback is intentionally never logged or persisted — losing it just
	// means every extension session must re-authenticate via a new bootstrap
	// code, which is the safe failure mode (never a predictable secret).
	extJWTSecret, err := extJWTSigningSecret(cfg.ExtJWTSecret)
	if err != nil {
		return err
	}
	extSigner := extauth.NewSigner(extJWTSecret)
	extAuthSvc := extauth.NewService(database.Queries, extSigner)
	extAuthHandler := &httpapi.ExtAuthHandler{Auth: extAuthSvc}
	extProfileHandler := &httpapi.ExtProfileHandler{Profiles: profileSvc, Verifier: extSigner}

	// Recruiter/hiring-manager resolution (007): posting-text always runs;
	// the company-page source reuses scrapingSvc the same way companyintel's
	// HeadcountScraper does, and LinkedIn only runs when the operator has
	// opted in via LINKEDIN_SCRAPE_ENABLED.
	recruiterSvc := recruiter.NewService(database.Queries, defaultRouter, "", scrapingSvc, cfg.LinkedInScrapeEnabled)
	contactsHandler := &httpapi.ContactsHandler{Recruiter: recruiterSvc}

	referralSvc := referral.NewService(database.Queries, database.Queries, referral.NewGitHubCrossReferencer())
	referralHandler := &httpapi.ReferralHandler{Referral: referralSvc}

	// Post-apply outreach draft generator (012): consumes 007's resolved
	// contacts (recruiterSvc) as the sole addressee source and 004's
	// company-intel signals (companyIntelSvc) as the sole grounding
	// source — it re-resolves neither, per assumptions.md. Draft-only: no
	// send path exists anywhere in this package (Principle I).
	outreachSvc := outreach.NewService(recruiterSvc, companyIntelSvc, defaultRouter, "")
	outreachHandler := &httpapi.OutreachHandler{Outreach: outreachSvc}

	router := httpapi.NewRouter(
		sourcesHandler.Mount, searchesHandler.Mount, documentsHandler.Mount,
		profilesHandler.Mount, jobsHandler.Mount, applicationsHandler.Mount,
		subsHandler.Mount, activityHandler.Mount, keywordHandler.Mount,
		postageHandler.Mount, notificationHandler.Mount, companiesHandler.Mount,
		ghostJobHandler.Mount, coachHandler.Mount,
		extAuthHandler.Mount, extProfileHandler.Mount,
		contactsHandler.Mount, referralHandler.Mount,
		outreachHandler.Mount, llmSettingsHandler.Mount, autoGenerateHandler.Mount,
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

	// Concurrency 1: same local-LLM contention reasoning as match/generate.
	ghostMux := asynq.NewServeMux()
	ghostMux.HandleFunc(queue.TypeGhostScore, ghostHandler.ProcessTask)
	ghostAsynqSrv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{queue.QueueGhostScore: 1},
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
	go func() {
		if err := ghostAsynqSrv.Run(ghostMux); err != nil {
			slog.Error("asynq ghost worker error", "error", err)
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
	ghostAsynqSrv.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
