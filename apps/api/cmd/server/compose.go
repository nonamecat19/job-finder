package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"

	"github.com/job-finder/api/internal/aifeature"
	"github.com/job-finder/api/internal/applications"
	"github.com/job-finder/api/internal/coach"
	"github.com/job-finder/api/internal/companyintel"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/extauth"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/interviewprep"
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
	"github.com/job-finder/api/internal/recruiter"
	"github.com/job-finder/api/internal/referral"
	"github.com/job-finder/api/internal/salary"
	"github.com/job-finder/api/internal/storage"
	"github.com/job-finder/api/internal/subscriptions"
)

// App is the fully wired set of HTTP and worker handlers plus the scheduler,
// consumed by buildServers (router mounting + worker muxes) and runServers.
type App struct {
	// HTTP handlers (each exposes Mount).
	Sources       *httpapi.SourcesHandler
	Searches      *httpapi.SearchesHandler
	Documents     *httpapi.DocumentsHandler
	Profiles      *httpapi.ProfilesHandler
	Jobs          *httpapi.JobsHandler
	Applications  *httpapi.ApplicationsHandler
	Subs          *httpapi.SubscriptionsHandler
	Activity      *httpapi.ActivityHandler
	Keyword       *httpapi.KeywordHandler
	PostAge       *httpapi.PostAgeHandler
	Notification  *httpapi.NotificationHandler
	Companies     *httpapi.CompaniesHandler
	GhostJob      *httpapi.GhostJobHandler
	Coach         *httpapi.CoachHandler
	ExtAuth       *httpapi.ExtAuthHandler
	ExtProfile    *httpapi.ExtProfileHandler
	Contacts      *httpapi.ContactsHandler
	Referral      *httpapi.ReferralHandler
	Outreach      *httpapi.OutreachHandler
	LlmSettings   *httpapi.LlmSettingsHandler
	AiFeatures    *httpapi.AiFeatureHandler
	InterviewPrep *httpapi.InterviewPrepHandler
	Health        *httpapi.HealthHandler

	// Worker handlers (each exposes ProcessTask).
	Ingestion  *ingestion.Handler
	Matching   *matching.Handler
	Generation *generation.Handler
	Enrichment *enrichment.Handler
	Salary     *salary.Handler
	Ghost      *ghostjob.Handler

	Scheduler *ingestion.Scheduler
}

type sourcesHandles struct {
	Registry  *jobsources.Registry
	Sources   *jobsources.Service
	Djinni    adapters.DjinniAdapter
	Dou       adapters.DouAdapter
	Workua    adapters.WorkUaAdapter
	Indeed    adapters.IndeedAdapter
	RemoteOK  adapters.RemoteOKAdapter
	Glassdoor adapters.GlassdoorAdapter
	JobLeads  adapters.JobLeadsAdapter
	Jobgether adapters.JobgetherAdapter
}

// composeJobSources builds the adapter registry and jobsources.Service, then
// wires the djinni session's Sources back-reference now that the service
// exists (closing the adapter<->service cycle).
func composeJobSources(p *Platform) *sourcesHandles {
	djinniAdapter := adapters.DjinniAdapter{Scraping: p.Scraping, Session: p.DjinniSession}
	douAdapter := adapters.DouAdapter{Scraping: p.Scraping}
	workuaAdapter := adapters.WorkUaAdapter{Scraping: p.Scraping}
	indeedAdapter := adapters.IndeedAdapter{Scraping: p.Scraping}
	remoteokAdapter := adapters.RemoteOKAdapter{Scraping: p.Scraping}
	glassdoorAdapter := adapters.GlassdoorAdapter{Scraping: p.Scraping}
	jobleadsAdapter := adapters.JobLeadsAdapter{Scraping: p.Scraping, Session: p.JobLeadsSession}
	jobgetherAdapter := adapters.JobgetherAdapter{Scraping: p.Scraping}
	registry := jobsources.NewRegistry(
		adapters.AdzunaAdapter{},
		adapters.RemotiveAdapter{},
		adapters.ArbeitnowAdapter{},
		djinniAdapter,
		douAdapter,
		workuaAdapter,
		indeedAdapter,
		remoteokAdapter,
		glassdoorAdapter,
		jobleadsAdapter,
		adapters.RobotaAdapter{},
		adapters.JoobleAdapter{},
		jobgetherAdapter,
	)
	sourcesSvc := jobsources.NewService(p.DB.Queries, registry, p.Config.ConfigEncryptionKey)
	p.DjinniSession.Sources = sourcesSvc
	p.JobLeadsSession.Sources = sourcesSvc
	return &sourcesHandles{
		Registry:  registry,
		Sources:   sourcesSvc,
		Djinni:    djinniAdapter,
		Dou:       douAdapter,
		Workua:    workuaAdapter,
		Indeed:    indeedAdapter,
		RemoteOK:  remoteokAdapter,
		Glassdoor: glassdoorAdapter,
		JobLeads:  jobleadsAdapter,
		Jobgether: jobgetherAdapter,
	}
}

type ingestionHandles struct {
	Ingestion *ingestion.Service
	Handler   *ingestion.Handler
	Scheduler *ingestion.Scheduler
	Sources   *httpapi.SourcesHandler
	Searches  *httpapi.SearchesHandler
}

func composeIngestion(p *Platform, sources *sourcesHandles) *ingestionHandles {
	ingestionSvc := ingestion.NewService(p.DB.Queries, sources.Registry, sources.Sources, p.AsynqClient)
	ingestionHandler := ingestion.NewHandler(p.DB.Queries, sources.Registry, sources.Sources, p.AsynqClient)
	scheduler := ingestion.NewScheduler(p.DB.Queries, ingestionSvc)
	return &ingestionHandles{
		Ingestion: ingestionSvc,
		Handler:   ingestionHandler,
		Scheduler: scheduler,
		Sources:   &httpapi.SourcesHandler{Sources: sources.Sources, Ingestion: ingestionSvc},
		Searches:  &httpapi.SearchesHandler{Ingestion: ingestionSvc},
	}
}

type llmHandles struct {
	Ollama           *llm.OllamaProvider
	MatchRouter      *llm.Router
	GenerationRouter *llm.Router
	RephraseRouter   *llm.Router
	GhostRouter      *llm.Router
	DefaultRouter    *llm.Router
	Settings         *llmsettings.Service
	SettingsHandler  *httpapi.LlmSettingsHandler
}

// composeLLM builds the shared providers and one llm.Router per named chat
// task, all sharing a single settings snapshot so a dashboard change takes
// effect for newly started tasks without a restart. Cerebras and OpenRouter
// are nil when unconfigured; each typed-nil is converted to a nil Provider
// interface so the routers see a genuinely absent provider.
func composeLLM(ctx context.Context, p *Platform) (*llmHandles, error) {
	ollamaProvider, cerebrasProvider, openRouterProvider, err := llm.NewProviders(p.Config)
	if err != nil {
		return nil, err
	}
	var cerebrasIface llm.Provider
	if cerebrasProvider != nil {
		cerebrasIface = cerebrasProvider
	}
	var openRouterIface llm.Provider
	if openRouterProvider != nil {
		openRouterIface = openRouterProvider
	}
	llmSettingsSvc, err := llmsettings.NewService(
		ctx, p.DB.Queries, p.Config.CerebrasAPIKey != "", p.Config.OpenRouterAPIKey != "",
	)
	if err != nil {
		return nil, err
	}
	llmHolder := llmSettingsSvc.Holder()
	return &llmHandles{
		Ollama:           ollamaProvider,
		MatchRouter:      llm.NewRouter("match", llmHolder, ollamaProvider, cerebrasIface, openRouterIface),
		GenerationRouter: llm.NewRouter("generation", llmHolder, ollamaProvider, cerebrasIface, openRouterIface),
		RephraseRouter:   llm.NewRouter("rephrase", llmHolder, ollamaProvider, cerebrasIface, openRouterIface),
		GhostRouter:      llm.NewRouter("ghost", llmHolder, ollamaProvider, cerebrasIface, openRouterIface),
		DefaultRouter:    llm.NewRouter("default", llmHolder, ollamaProvider, cerebrasIface, openRouterIface),
		Settings:         llmSettingsSvc,
		SettingsHandler:  &httpapi.LlmSettingsHandler{Settings: llmSettingsSvc},
	}, nil
}

type profileHandles struct {
	Profile *profile.Service
	Handler *httpapi.ProfilesHandler
}

// composeProfile wires profile.Service to Ollama directly (it only ever
// embeds, never chats, so it needs no task Router).
func composeProfile(p *Platform, ollama *llm.OllamaProvider) *profileHandles {
	profileSvc := profile.NewService(p.DB.Queries, ollama, p.Config.EmbedModel, p.Config.RendercvBin)
	return &profileHandles{
		Profile: profileSvc,
		Handler: &httpapi.ProfilesHandler{Profiles: profileSvc},
	}
}

type matchingHandles struct {
	Jobs             *jobs.Service
	Notifier         *notifier.Service
	AiFeatures       *aifeature.Service
	AiFeatureHandler *httpapi.AiFeatureHandler
	Handler          *matching.Handler
}

// composeMatching also owns jobs.Service: matchingHandler auto-enqueues
// resume/cover-letter generation and salary inference via it when a job's
// score crosses that feature's configurable threshold, so it must exist
// before the matching handler.
func composeMatching(ctx context.Context, p *Platform, profileSvc *profile.Service, matchRouter *llm.Router) (*matchingHandles, error) {
	matchingSvc := matching.NewService(p.DB.Queries, profileSvc, matchRouter, p.Config.MatchSimilarityThreshold, "")
	notifierSvc := notifier.NewService(p.DB.Queries,
		notifier.WithMatchThreshold(p.Config.MatchNotifyScoreThreshold),
		notifier.WithRateLimitCap(p.Config.MatchNotifyRateLimit),
	)
	jobsSvc := jobs.NewService(p.DB.Queries, p.AsynqClient, p.Config.SalaryFloorUsd)
	aiFeatureSvc, err := aifeature.NewService(ctx, p.DB.Queries)
	if err != nil {
		return nil, err
	}
	return &matchingHandles{
		Jobs:             jobsSvc,
		Notifier:         notifierSvc,
		AiFeatures:       aiFeatureSvc,
		AiFeatureHandler: &httpapi.AiFeatureHandler{Settings: aiFeatureSvc},
		Handler:          matching.NewHandler(matchingSvc, notifierSvc, aiFeatureSvc, jobsSvc, jobsSvc),
	}, nil
}

type ghostHandles struct {
	Worker      *ghostjob.Handler
	HTTPHandler *httpapi.GhostJobHandler
}

// composeGhostJob builds the ghost-job detector, kept separate from
// matching/fit scoring end-to-end.
func composeGhostJob(p *Platform, ghostRouter *llm.Router) *ghostHandles {
	ghostSvc := ghostjob.NewService(p.DB.Queries, ghostRouter, "")
	return &ghostHandles{
		Worker:      ghostjob.NewHandler(ghostSvc, p.DB.Queries),
		HTTPHandler: &httpapi.GhostJobHandler{Ghost: ghostSvc},
	}
}

type generationHandles struct {
	Generation *generation.Service
	Handler    *generation.Handler
	Documents  *httpapi.DocumentsHandler
}

// composeGeneration wires the renderers (and their optional MinIO blob store)
// and generation.Service.
func composeGeneration(ctx context.Context, p *Platform, profileSvc *profile.Service, generationRouter *llm.Router) (*generationHandles, error) {
	cfg := p.Config

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
			return nil, err
		}
		blobStore = ms
		slog.Info("MinIO object storage enabled", "endpoint", cfg.MinioEndpoint, "bucket", cfg.MinioBucket)
	}

	htmlRenderer, err := generation.NewHtmlPdfRenderer(p.Scraping, cfg.DocumentsDir)
	if err != nil {
		return nil, err
	}
	htmlRenderer.Store = blobStore
	rendercvRenderer := generation.NewRenderCvRenderer(cfg.DocumentsDir, cfg.RendercvBin)
	rendercvRenderer.Store = blobStore
	generationSvc := generation.NewService(p.DB.Queries, profileSvc, htmlRenderer, rendercvRenderer, generationRouter, "", cfg.ResumeMasterPath, cfg.ResumeGroundingLvl)
	return &generationHandles{
		Generation: generationSvc,
		Handler:    generation.NewHandler(generationSvc),
		Documents:  &httpapi.DocumentsHandler{Generation: generationSvc},
	}, nil
}

func composeApplications(p *Platform) *httpapi.ApplicationsHandler {
	// p.DB (not p.DB.Queries) is passed as the TxRunner so a status change and
	// its "ApplicationOutcome" event commit atomically.
	applicationsSvc := applications.NewService(p.DB.Queries, p.DB)
	return &httpapi.ApplicationsHandler{Applications: applicationsSvc}
}

func composeSubscriptions(p *Platform, sourcesSvc *jobsources.Service, ingestionSvc *ingestion.Service) *httpapi.SubscriptionsHandler {
	subsSvc := subscriptions.NewService(p.DB.Queries, sourcesSvc)
	return &httpapi.SubscriptionsHandler{Subs: subsSvc, Ingestion: ingestionSvc}
}

// composeEnrichment builds the enrichment worker handler, reusing the same
// adapter values as the registry so it hits the shared djinni session.
func composeEnrichment(p *Platform, sources *sourcesHandles) *enrichment.Handler {
	cfg := p.Config
	enrichDelay := time.Duration(cfg.DjinniDetailDelayMs) * time.Millisecond
	enrichDelays := map[string]time.Duration{
		"workua": time.Duration(cfg.WorkUaDetailDelayMs) * time.Millisecond,
	}
	return enrichment.NewHandler(p.DB.Queries, sources.Sources, sources.Djinni, sources.Dou, sources.Workua, sources.Indeed, sources.RemoteOK, sources.Glassdoor, sources.JobLeads, sources.Jobgether, p.AsynqClient, enrichDelay, enrichDelays)
}

type salaryHandles struct {
	Worker       *salary.Handler
	LevelsLoader *salary.LevelsFyiLoader
}

// composeSalary builds the salary inference worker and loads the levels.fyi
// CSV when configured (a warn-only side effect, unchanged from the original).
func composeSalary(ctx context.Context, p *Platform, defaultRouter *llm.Router) *salaryHandles {
	cfg := p.Config
	levelsFyiLoader := salary.NewLevelsFyiLoader(p.DB.Queries)
	salaryService := salary.NewService(p.DB.Queries, defaultRouter, levelsFyiLoader, "")

	if cfg.LevelsFyiCSV != "" {
		if _, err := levelsFyiLoader.LoadCSV(ctx, cfg.LevelsFyiCSV); err != nil {
			slog.Warn("salary: levels.fyi CSV load failed", "path", cfg.LevelsFyiCSV, "error", err)
		}
	} else {
		slog.Warn("salary: LEVELS_FYI_CSV not set — levels.fyi source disabled")
	}

	return &salaryHandles{
		Worker:       salary.NewHandler(salaryService, p.DB.Queries),
		LevelsLoader: levelsFyiLoader,
	}
}

type keywordHandles struct {
	Handler       *httpapi.KeywordHandler
	RephraseModel *keyword.ProviderRephraseModel
}

// composeKeyword builds the keyword-diff endpoint with its async, TTL'd
// rephrase cache. RephraseModel is returned so the fit-gap coach can reuse the
// identical truthful-reframing port.
func composeKeyword(p *Platform, rephraseRouter *llm.Router, profileSvc *profile.Service) *keywordHandles {
	rephraseModel := keyword.NewProviderRephraseModel(rephraseRouter, "")
	cachedRephraser := keyword.NewCachedRephraser(
		keyword.NewSuggester(rephraseModel),
		time.Duration(p.Config.KeywordRephraseCacheTTLSec)*time.Second,
	)
	diffService := keyword.NewDiffService(p.DB.Queries).WithRephraser(cachedRephraser, profileSvc)
	return &keywordHandles{
		Handler:       &httpapi.KeywordHandler{Diff: diffService},
		RephraseModel: rephraseModel,
	}
}

// composeCoach builds the fit-gap coach. ProfileEntries closes over profileSvc
// rather than coach importing internal/profile, keeping that edge one-directional.
func composeCoach(p *Platform, rephraseModel *keyword.ProviderRephraseModel, profileSvc *profile.Service) *httpapi.CoachHandler {
	coachSvc := coach.NewService(rephraseModel)
	coachAssessSvc := coach.NewAssessmentService(coachSvc, p.DB.Queries, func(ctx context.Context) ([]coach.ProfileEntry, error) {
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
	return &httpapi.CoachHandler{Coach: coachAssessSvc}
}

// composeInterviewPrep builds the interview prep pack (013). Stories closes
// over profileSvc rather than interviewprep importing internal/profile,
// mirroring composeCoach's ProfileEntries closure.
func composeInterviewPrep(p *Platform, profileSvc *profile.Service, companyIntelSvc *companyintel.Service) *httpapi.InterviewPrepHandler {
	prepSvc := interviewprep.NewService(p.DB.Queries, p.DB.Queries, func(ctx context.Context) ([]keyword.StarStory, error) {
		prof, err := profileSvc.GetDefault(ctx)
		if err != nil {
			// No profile yet is not an error — there is simply nothing to match.
			return nil, nil
		}
		rows, err := p.DB.Queries.ListStarStoriesByProfile(ctx, prof.ID)
		if err != nil {
			return nil, err
		}
		out := make([]keyword.StarStory, 0, len(rows))
		for _, row := range rows {
			var skills []string
			_ = json.Unmarshal(row.Skills, &skills)
			var categories []keyword.StoryCategory
			_ = json.Unmarshal(row.Categories, &categories)
			out = append(out, keyword.StarStory{
				ID:         dbutil.UUIDString(row.ID),
				ProfileID:  dbutil.UUIDString(row.ProfileId),
				Title:      row.Title,
				Situation:  row.Situation,
				Task:       row.Task,
				Action:     row.Action,
				Result:     row.Result,
				Skills:     skills,
				Categories: categories,
			})
		}
		return out, nil
	}, companyIntelSvc)
	return &httpapi.InterviewPrepHandler{InterviewPrep: prepSvc}
}

func composePostAge(p *Platform) *httpapi.PostAgeHandler {
	postageSvc := postage.NewService(p.DB.Queries)
	return &httpapi.PostAgeHandler{PostAge: postageSvc}
}

func composeNotifications(p *Platform) *httpapi.NotificationHandler {
	notificationSvc := notifier.NewNotificationService(p.DB.Queries, p.DB.Queries)
	return &httpapi.NotificationHandler{Provider: notificationSvc}
}

type companyIntelHandles struct {
	Service *companyintel.Service
	Handler *httpapi.CompaniesHandler
}

func composeCompanyIntel(p *Platform) *companyIntelHandles {
	companyIntelRegistry := companyintel.NewRegistry(
		companyintel.CrunchbaseScraper{Scraping: p.Scraping},
		companyintel.LayoffsScraper{Scraping: p.Scraping},
		companyintel.GlassdoorScraper{Scraping: p.Scraping},
		companyintel.HeadcountScraper{Scraping: p.Scraping},
		companyintel.TechStackScraper{Scraping: p.Scraping},
	)
	companyIntelSvc := companyintel.NewService(p.DB.Queries, companyIntelRegistry, 2*time.Second)
	return &companyIntelHandles{
		Service: companyIntelSvc,
		Handler: &httpapi.CompaniesHandler{CompanyIntel: companyIntelSvc},
	}
}

type extAuthHandles struct {
	Auth    *httpapi.ExtAuthHandler
	Profile *httpapi.ExtProfileHandler
}

// composeExtAuth builds browser-extension auth. extJWTSecret must be a random
// byte string; the hex config value is decoded when present, or a fresh one is
// generated per process start when absent.
func composeExtAuth(p *Platform, profileSvc *profile.Service) (*extAuthHandles, error) {
	extJWTSecret, err := extJWTSigningSecret(p.Config.ExtJWTSecret)
	if err != nil {
		return nil, err
	}
	extSigner := extauth.NewSigner(extJWTSecret)
	extAuthSvc := extauth.NewService(p.DB.Queries, extSigner)
	return &extAuthHandles{
		Auth:    &httpapi.ExtAuthHandler{Auth: extAuthSvc},
		Profile: &httpapi.ExtProfileHandler{Profiles: profileSvc, Verifier: extSigner},
	}, nil
}

type recruiterHandles struct {
	Service *recruiter.Service
	Handler *httpapi.ContactsHandler
}

// composeRecruiter builds recruiter/hiring-manager resolution. LinkedIn only
// runs when the operator has opted in via LINKEDIN_SCRAPE_ENABLED.
func composeRecruiter(p *Platform, defaultRouter *llm.Router) *recruiterHandles {
	recruiterSvc := recruiter.NewService(p.DB.Queries, defaultRouter, "", p.Scraping, p.Config.LinkedInScrapeEnabled)
	return &recruiterHandles{
		Service: recruiterSvc,
		Handler: &httpapi.ContactsHandler{Recruiter: recruiterSvc},
	}
}

func composeReferral(p *Platform) *httpapi.ReferralHandler {
	referralSvc := referral.NewService(p.DB.Queries, p.DB.Queries, referral.NewGitHubCrossReferencer())
	return &httpapi.ReferralHandler{Referral: referralSvc}
}

// composeOutreach builds the post-apply outreach draft generator. It consumes
// 007's resolved contacts and 004's company-intel signals as its sole sources,
// re-resolving neither. Draft-only: no send path exists.
func composeOutreach(p *Platform, recruiterSvc *recruiter.Service, companyIntelSvc *companyintel.Service, defaultRouter *llm.Router) *httpapi.OutreachHandler {
	outreachSvc := outreach.NewService(recruiterSvc, companyIntelSvc, defaultRouter, "")
	return &httpapi.OutreachHandler{Outreach: outreachSvc}
}

// buildContexts runs every feature composer in construction order and performs
// the two cross-composer wires that cannot live inside a single composer:
// jobsHandler (jobs.Service from matching + generation.Service) and the
// SourcesHandler.Enrichment back-reference.
func buildContexts(ctx context.Context, p *Platform) (*App, error) {
	sources := composeJobSources(p)
	ingestionH := composeIngestion(p, sources)

	llmH, err := composeLLM(ctx, p)
	if err != nil {
		return nil, err
	}
	profileH := composeProfile(p, llmH.Ollama)

	matchingH, err := composeMatching(ctx, p, profileH.Profile, llmH.MatchRouter)
	if err != nil {
		return nil, err
	}
	ghostH := composeGhostJob(p, llmH.GhostRouter)

	generationH, err := composeGeneration(ctx, p, profileH.Profile, llmH.GenerationRouter)
	if err != nil {
		return nil, err
	}
	jobsHandler := &httpapi.JobsHandler{Jobs: matchingH.Jobs, Generation: generationH.Generation}

	enrichHandler := composeEnrichment(p, sources)
	ingestionH.Sources.Enrichment = enrichHandler

	salaryH := composeSalary(ctx, p, llmH.DefaultRouter)

	extAuthH, err := composeExtAuth(p, profileH.Profile)
	if err != nil {
		return nil, err
	}

	keywordH := composeKeyword(p, llmH.RephraseRouter, profileH.Profile)
	companyIntelH := composeCompanyIntel(p)
	recruiterH := composeRecruiter(p, llmH.DefaultRouter)

	return &App{
		Sources:       ingestionH.Sources,
		Searches:      ingestionH.Searches,
		Documents:     generationH.Documents,
		Profiles:      profileH.Handler,
		Jobs:          jobsHandler,
		Applications:  composeApplications(p),
		Subs:          composeSubscriptions(p, sources.Sources, ingestionH.Ingestion),
		Activity:      httpapi.NewActivityHandler(p.DB.Queries, p.AsynqClient, p.AsynqInspector),
		Keyword:       keywordH.Handler,
		PostAge:       composePostAge(p),
		Notification:  composeNotifications(p),
		Companies:     companyIntelH.Handler,
		GhostJob:      ghostH.HTTPHandler,
		Coach:         composeCoach(p, keywordH.RephraseModel, profileH.Profile),
		ExtAuth:       extAuthH.Auth,
		ExtProfile:    extAuthH.Profile,
		Contacts:      recruiterH.Handler,
		Referral:      composeReferral(p),
		Outreach:      composeOutreach(p, recruiterH.Service, companyIntelH.Service, llmH.DefaultRouter),
		LlmSettings:   llmH.SettingsHandler,
		AiFeatures:    matchingH.AiFeatureHandler,
		InterviewPrep: composeInterviewPrep(p, profileH.Profile, companyIntelH.Service),
		Health:        composeHealth(p),

		Ingestion:  ingestionH.Handler,
		Matching:   matchingH.Handler,
		Generation: generationH.Handler,
		Enrichment: enrichHandler,
		Salary:     salaryH.Worker,
		Ghost:      ghostH.Worker,

		Scheduler: ingestionH.Scheduler,
	}, nil
}

// composeHealth wires the readiness endpoint's dependency pings. p.DB.Pool
// already satisfies httpapi.Pinger directly (pgxpool.Pool.Ping(ctx) error);
// Redis and MinIO need small adapters since their native clients don't match
// that exact signature.
func composeHealth(p *Platform) *httpapi.HealthHandler {
	var minioPing httpapi.Pinger
	if p.MinioReady != nil {
		minioPing = minioPinger{client: p.MinioReady, bucket: p.Config.MinioBucket}
	}
	return &httpapi.HealthHandler{
		Postgres: p.DB.Pool,
		Redis:    redisPinger{client: p.RedisClient},
		Minio:    minioPing,
	}
}

type redisPinger struct{ client redis.UniversalClient }

func (r redisPinger) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

type minioPinger struct {
	client *minio.Client
	bucket string
}

func (m minioPinger) Ping(ctx context.Context) error {
	_, err := m.client.BucketExists(ctx, m.bucket)
	return err
}

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
