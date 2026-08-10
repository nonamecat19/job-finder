package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	activityhttp "github.com/job-finder/api/internal/activity/interfaces/http"
	"github.com/job-finder/api/internal/aifeature"
	aifeaturehttp "github.com/job-finder/api/internal/aifeature/interfaces/http"
	"github.com/job-finder/api/internal/applications"
	applicationshttp "github.com/job-finder/api/internal/applications/interfaces/http"
	"github.com/job-finder/api/internal/coach"
	coachhttp "github.com/job-finder/api/internal/coach/interfaces/http"
	"github.com/job-finder/api/internal/companyintel"
	companyintelhttp "github.com/job-finder/api/internal/companyintel/interfaces/http"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/generation"
	generationhttp "github.com/job-finder/api/internal/generation/interfaces/http"
	"github.com/job-finder/api/internal/ghostjob"
	ghostjobhttp "github.com/job-finder/api/internal/ghostjob/interfaces/http"
	"github.com/job-finder/api/internal/health"
	"github.com/job-finder/api/internal/interviewprep"
	interviewprephttp "github.com/job-finder/api/internal/interviewprep/interfaces/http"
	"github.com/job-finder/api/internal/jobs"
	jobshttp "github.com/job-finder/api/internal/jobs/interfaces/http"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/jobsources/infrastructure/adapters"
	jobsourceshttp "github.com/job-finder/api/internal/jobsources/interfaces/http"
	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
	"github.com/job-finder/api/internal/jobsources/roster"
	"github.com/job-finder/api/internal/keyword"
	keywordhttp "github.com/job-finder/api/internal/keyword/interfaces/http"
	"github.com/job-finder/api/internal/manualadd"
	manualaddhttp "github.com/job-finder/api/internal/manualadd/interfaces/http"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/notifier"
	notifierhttp "github.com/job-finder/api/internal/notifier/interfaces/http"
	"github.com/job-finder/api/internal/outreach"
	outreachhttp "github.com/job-finder/api/internal/outreach/interfaces/http"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/platform/observability"
	"github.com/job-finder/api/internal/platform/storage"
	"github.com/job-finder/api/internal/postage"
	postagehttp "github.com/job-finder/api/internal/postage/interfaces/http"
	"github.com/job-finder/api/internal/profile"
	profilehttp "github.com/job-finder/api/internal/profile/interfaces/http"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/recruiter"
	recruiterhttp "github.com/job-finder/api/internal/recruiter/interfaces/http"
	"github.com/job-finder/api/internal/referral"
	referralhttp "github.com/job-finder/api/internal/referral/interfaces/http"
	"github.com/job-finder/api/internal/resumeshape"
	resumeshapehttp "github.com/job-finder/api/internal/resumeshape/interfaces/http"
	"github.com/job-finder/api/internal/retrieval"
	"github.com/job-finder/api/internal/salary"
	"github.com/job-finder/api/internal/subscriptions"
	subscriptionshttp "github.com/job-finder/api/internal/subscriptions/interfaces/http"
	"github.com/job-finder/api/internal/summarymodel"
	summarymodelhttp "github.com/job-finder/api/internal/summarymodel/interfaces/http"
)

type App struct {
	Sources       *jobsourceshttp.SourcesHandler
	Roster        *jobsourceshttp.RosterHandler
	Searches      *jobsourceshttp.SearchesHandler
	Documents     *generationhttp.DocumentsHandler
	Profiles      *profilehttp.ProfilesHandler
	Jobs          *jobshttp.JobsHandler
	Applications  *applicationshttp.ApplicationsHandler
	Subs          *subscriptionshttp.SubscriptionsHandler
	ManualAdd     *manualaddhttp.ManualAddHandler
	Activity      *activityhttp.ActivityHandler
	Keyword       *keywordhttp.KeywordHandler
	PostAge       *postagehttp.PostAgeHandler
	Notification  *notifierhttp.NotificationHandler
	Companies     *companyintelhttp.CompaniesHandler
	GhostJob      *ghostjobhttp.GhostJobHandler
	Coach         *coachhttp.CoachHandler
	Contacts      *recruiterhttp.ContactsHandler
	Referral      *referralhttp.ReferralHandler
	Outreach      *outreachhttp.OutreachHandler
	AiFeatures    *aifeaturehttp.AiFeatureHandler
	ResumeShape   *resumeshapehttp.ResumeShapeHandler
	SummaryModel  *summarymodelhttp.SummaryModelHandler
	InterviewPrep *interviewprephttp.InterviewPrepHandler
	Health        *health.HealthHandler
	Hosts         *jobsourceshttp.HostsHandler

	Ingestion  *worker.Handler
	Matching   *matching.Handler
	Generation *generation.Handler
	Enrichment *enrichment.Handler
	Salary     *salary.Handler
	Ghost      *ghostjob.Handler

	MatchRouter      *llm.Router
	GenerationRouter *llm.Router
	DefaultRouter    *llm.Router
	GhostRouter      *llm.Router

	Scheduler *worker.Scheduler
}

type sourcesHandles struct {
	Registry *domain.Registry
	Sources  *application.Service
	Djinni   adapters.DjinniAdapter
	Dou      adapters.DouAdapter
	Workua   adapters.WorkUaAdapter
	Roster   *roster.Service
}

func composeJobSources(p *Platform) *sourcesHandles {
	djinniAdapter := adapters.DjinniAdapter{Scraping: p.Scraping, Session: p.DjinniSession}
	douAdapter := adapters.DouAdapter{Scraping: p.Scraping}
	workuaAdapter := adapters.WorkUaAdapter{Scraping: p.Scraping}

	ghAdapter, lvAdapter, asAdapter, wkAdapter, srAdapter, checkers := adapters.NewBoardAdapters()
	rosterSvc := roster.NewService(p.DB.Queries, checkers)
	ghAdapter.Roster = rosterSvc
	lvAdapter.Roster = rosterSvc
	asAdapter.Roster = rosterSvc
	wkAdapter.Roster = rosterSvc
	srAdapter.Roster = rosterSvc

	registry := domain.NewRegistry(
		adapters.AdzunaAdapter{AppID: p.Config.AdzunaAppID, AppKey: p.Config.AdzunaAppKey, Country: p.Config.AdzunaCountry},
		adapters.RemotiveAdapter{},
		adapters.ArbeitnowAdapter{},
		djinniAdapter,
		douAdapter,
		workuaAdapter,
		adapters.RobotaAdapter{},
		adapters.JobSpyAdapter{URL: p.Config.JobspyURL},
		adapters.JoobleAdapter{APIKey: p.Config.JoobleAPIKey},
		ghAdapter,
		lvAdapter,
		asAdapter,
		wkAdapter,
		srAdapter,
		adapters.ManualAdapter{},
	)
	sourcesSvc := application.NewService(p.DB.Queries, registry, p.Config.ConfigEncryptionKey)
	p.DjinniSession.Sources = sourcesSvc
	return &sourcesHandles{
		Registry: registry,
		Sources:  sourcesSvc,
		Djinni:   djinniAdapter,
		Dou:      douAdapter,
		Workua:   workuaAdapter,
		Roster:   rosterSvc,
	}
}

type ingestionHandles struct {
	Ingestion *application.SearchService
	Handler   *worker.Handler
	Scheduler *worker.Scheduler
	Sources   *jobsourceshttp.SourcesHandler
	Searches  *jobsourceshttp.SearchesHandler
}

func composeIngestion(p *Platform, sources *sourcesHandles) *ingestionHandles {
	ingestionSvc := application.NewSearchService(p.DB.Queries, sources.Registry, sources.Sources, p.AsynqClient)
	ingestionHandler := worker.NewHandler(p.DB.Queries, sources.Registry, sources.Sources, p.AsynqClient,
		worker.WithTxRunner(p.DB),
		worker.WithChunkSize(p.Config.IngestPersistChunkSize),
	)
	// The retention job for the LLM observability collector (036 FR-008) rides
	// on the ingestion scheduler's tick, which is the platform's existing
	// periodic-maintenance loop. It no-ops when the collector is unconfigured,
	// which is the default.
	pruner := observability.New(observability.Config{
		BaseURL:       p.Config.EvalPruneCollectorURL,
		PublicKey:     p.Config.EvalPrunePublicKey,
		SecretKey:     p.Config.EvalPruneSecretKey,
		RetentionDays: p.Config.EvalPruneRetentionDay,
	}, nil)
	scheduler := worker.NewScheduler(p.DB.Queries, ingestionSvc, pruner)
	return &ingestionHandles{
		Ingestion: ingestionSvc,
		Handler:   ingestionHandler,
		Scheduler: scheduler,
		Sources:   &jobsourceshttp.SourcesHandler{Sources: sources.Sources, Ingestion: ingestionSvc},
		Searches:  &jobsourceshttp.SearchesHandler{Ingestion: ingestionSvc},
	}
}

type llmHandles struct {
	Ollama      *llm.OllamaProvider
	MatchRouter *llm.Router
	// GenerationRouter serves the cover letter, which is still a single call
	// against the `generation` task key. Resume generation itself is split
	// across the stage routers below (035).
	GenerationRouter        *llm.Router
	GenerationAnalyzeRouter *llm.Router
	GenerationSelectRouter  *llm.Router
	GenerationPremiumRouter *llm.Router
	GenerationSummaryRouter *llm.Router
	// SummaryOptionRouters is one router per 034 summary option, keyed by
	// option id. Built from the catalogue so adding an option is a one-line
	// change there rather than a wiring change here.
	SummaryOptionRouters map[string]llm.Provider
	RephraseRouter       *llm.Router
	GhostRouter          *llm.Router
	DefaultRouter        *llm.Router
}

func composeLLM(p *Platform) (*llmHandles, error) {
	ollamaProvider, gatewayProvider, err := llm.NewProviders(p.Config)
	if err != nil {
		return nil, err
	}
	var gatewayIface llm.Provider
	if gatewayProvider != nil {
		gatewayIface = gatewayProvider
	}
	cfg := p.Config
	return &llmHandles{
		Ollama:                  ollamaProvider,
		MatchRouter:             llm.NewRouter("match", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelMatch)),
		GenerationRouter:        llm.NewRouter("generation", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGeneration)),
		GenerationAnalyzeRouter: llm.NewRouter("generation-analyze", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGeneration)),
		GenerationSelectRouter:  llm.NewRouter("generation-select", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGeneration)),
		GenerationPremiumRouter: llm.NewRouter("generation-select-premium", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGeneration)),
		GenerationSummaryRouter: llm.NewRouter("generation-summary", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGeneration)),
		SummaryOptionRouters:    summaryOptionRouters(gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGeneration)),
		RephraseRouter:          llm.NewRouter("rephrase", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelRephrase)),
		GhostRouter:             llm.NewRouter("ghost", gatewayIface, ollamaProvider, cfg.ModelOr(cfg.LLMModelGhost)),
		DefaultRouter:           llm.NewRouter("default", gatewayIface, ollamaProvider, cfg.LLMModel),
	}, nil
}

type profileHandles struct {
	Profile *profile.Service
	Handler *profilehttp.ProfilesHandler
}

func composeProfile(p *Platform, ollama *llm.OllamaProvider) *profileHandles {
	profileSvc := profile.NewService(p.DB.Queries, ollama, p.Config.EmbedModel, p.Config.RendercvBin)
	return &profileHandles{
		Profile: profileSvc,
		Handler: &profilehttp.ProfilesHandler{Profiles: profileSvc},
	}
}

type matchingHandles struct {
	Jobs             *jobs.Service
	Notifier         *notifier.Service
	AiFeatures       *aifeature.Service
	AiFeatureHandler *aifeaturehttp.AiFeatureHandler
	Handler          *matching.Handler
}

type resumeAutoGenerateGate struct{ svc *aifeature.Service }

func (g resumeAutoGenerateGate) ShouldGenerate(score int) bool {
	return g.svc.ShouldRun(aifeature.Resume, score)
}

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
		AiFeatureHandler: &aifeaturehttp.AiFeatureHandler{Settings: aiFeatureSvc},
		Handler:          matching.NewHandler(matchingSvc, notifierSvc, resumeAutoGenerateGate{aiFeatureSvc}, jobsSvc),
	}, nil
}

type ghostHandles struct {
	Worker      *ghostjob.Handler
	HTTPHandler *ghostjobhttp.GhostJobHandler
}

func composeGhostJob(p *Platform, ghostRouter *llm.Router) *ghostHandles {
	ghostSvc := ghostjob.NewService(p.DB.Queries, ghostRouter, "")
	return &ghostHandles{
		Worker:      ghostjob.NewHandler(ghostSvc, p.DB.Queries),
		HTTPHandler: &ghostjobhttp.GhostJobHandler{Ghost: ghostSvc},
	}
}

type generationHandles struct {
	Generation   *generation.Service
	Handler      *generation.Handler
	Documents    *generationhttp.DocumentsHandler
	ResumeShape  *resumeshapehttp.ResumeShapeHandler
	SummaryModel *summarymodelhttp.SummaryModelHandler
}

func composeGeneration(ctx context.Context, p *Platform, profileSvc *profile.Service, routers generation.Routers) (*generationHandles, error) {
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
	shapeSvc, err := resumeshape.NewService(ctx, p.DB.Queries)
	if err != nil {
		return nil, err
	}
	// 034: which option writes the summary. Like the shape service, a failure
	// to load it is not fatal — the catalogue default is a complete answer, and
	// refusing to start the API over a preferences row would be a poor trade.
	summarySvc, err := summarymodel.NewService(ctx, p.DB.Queries)
	if err != nil {
		return nil, err
	}
	generationSvc := generation.NewService(p.DB.Queries, profileSvc, htmlRenderer, rendercvRenderer, routers, "", cfg.ResumeMasterPath, cfg.ResumeGroundingLvl, shapeSvc)
	generationSvc.SetSummaryModelProvider(summarySvc)
	return &generationHandles{
		Generation:   generationSvc,
		Handler:      generation.NewHandler(generationSvc, p.DB.Queries),
		Documents:    &generationhttp.DocumentsHandler{Generation: generationSvc, SummaryModel: summarySvc},
		ResumeShape:  &resumeshapehttp.ResumeShapeHandler{Settings: shapeSvc},
		SummaryModel: &summarymodelhttp.SummaryModelHandler{Settings: summarySvc},
	}, nil
}

func composeApplications(p *Platform) *applicationshttp.ApplicationsHandler {
	applicationsSvc := applications.NewService(p.DB.Queries, p.DB)
	return &applicationshttp.ApplicationsHandler{Applications: applicationsSvc}
}

func composeSubscriptions(p *Platform, sourcesSvc *application.Service, ingestionSvc *application.SearchService) *subscriptionshttp.SubscriptionsHandler {
	subsSvc := subscriptions.NewService(p.DB.Queries, sourcesSvc)
	return &subscriptionshttp.SubscriptionsHandler{Subs: subsSvc, Ingestion: ingestionSvc}
}

// composeManualAdd wires the synchronous add path. It shares the subscription
// service with the CRUD endpoints so "at most one manual row per source" is one
// rule in one place, and the ingest package with the crawl worker so a manual
// vacancy is byte-for-byte what a crawl produces.
func composeManualAdd(p *Platform, sources *sourcesHandles, jobsSvc *jobs.Service) *manualaddhttp.ManualAddHandler {
	subsSvc := subscriptions.NewService(p.DB.Queries, sources.Sources)
	manualSvc := manualadd.NewService(
		p.DB.Queries, sources.Sources, subsSvc, sources.Registry, jobsSvc, p.AsynqClient,
		manualadd.WithTxRunner(p.DB),
		// User Story 3 has shipped, so no_reader and incomplete answer with a
		// draft rather than dead-ending (FR-019, FR-023).
		manualadd.WithFillIn(),
	)
	return &manualaddhttp.ManualAddHandler{Manual: manualSvc}
}

func composeEnrichment(p *Platform, sources *sourcesHandles, retrievalSvc retrieval.Service) *enrichment.Handler {
	cfg := p.Config
	enrichDelay := time.Duration(cfg.DjinniDetailDelayMs) * time.Millisecond
	enrichDelays := map[string]time.Duration{
		"workua": time.Duration(cfg.WorkUaDetailDelayMs) * time.Millisecond,
	}
	return enrichment.NewHandler(p.DB.Queries, sources.Sources, sources.Djinni, sources.Dou, sources.Workua,
		adapters.IndeedAdapter{Scraping: p.Scraping},
		adapters.RemoteOKAdapter{Scraping: p.Scraping},
		adapters.GlassdoorAdapter{Scraping: p.Scraping, Retrieval: retrievalSvc},
		adapters.JobLeadsAdapter{Scraping: p.Scraping, Session: &adapters.JobLeadsSession{Email: p.Config.JobLeadsEmail, Password: p.Config.JobLeadsPassword, Key: "jobleads"}},
		adapters.WellfoundAdapter{Scraping: p.Scraping, Retrieval: retrievalSvc},
		adapters.JobgetherAdapter{Scraping: p.Scraping, Retrieval: retrievalSvc},
		p.AsynqClient, enrichDelay, enrichDelays)
}

type salaryHandles struct {
	Worker       *salary.Handler
	LevelsLoader *salary.LevelsFyiLoader
}

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
	Handler       *keywordhttp.KeywordHandler
	RephraseModel *keyword.ProviderRephraseModel
}

func composeKeyword(p *Platform, rephraseRouter *llm.Router, profileSvc *profile.Service) *keywordHandles {
	rephraseModel := keyword.NewProviderRephraseModel(rephraseRouter, "")
	cachedRephraser := keyword.NewCachedRephraser(
		keyword.NewSuggester(rephraseModel),
		time.Duration(p.Config.KeywordRephraseCacheTTLSec)*time.Second,
	)
	diffService := keyword.NewDiffService(p.DB.Queries).WithRephraser(cachedRephraser, profileSvc)
	return &keywordHandles{
		Handler:       &keywordhttp.KeywordHandler{Diff: diffService},
		RephraseModel: rephraseModel,
	}
}

func composeCoach(p *Platform, rephraseModel *keyword.ProviderRephraseModel, profileSvc *profile.Service) *coachhttp.CoachHandler {
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
	return &coachhttp.CoachHandler{Coach: coachAssessSvc}
}

func composePostAge(p *Platform) *postagehttp.PostAgeHandler {
	postageSvc := postage.NewService(p.DB.Queries)
	return &postagehttp.PostAgeHandler{PostAge: postageSvc}
}

func composeNotifications(p *Platform) *notifierhttp.NotificationHandler {
	notificationSvc := notifier.NewNotificationService(p.DB.Queries, p.DB.Queries)
	return &notifierhttp.NotificationHandler{Provider: notificationSvc}
}

type companyIntelHandles struct {
	Service *companyintel.Service
	Handler *companyintelhttp.CompaniesHandler
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
		Handler: &companyintelhttp.CompaniesHandler{CompanyIntel: companyIntelSvc},
	}
}

type recruiterHandles struct {
	Service *recruiter.Service
	Handler *recruiterhttp.ContactsHandler
}

func composeRecruiter(p *Platform, defaultRouter *llm.Router) *recruiterHandles {
	recruiterSvc := recruiter.NewService(p.DB.Queries, defaultRouter, "", p.Scraping, p.Config.LinkedInScrapeEnabled)
	return &recruiterHandles{
		Service: recruiterSvc,
		Handler: &recruiterhttp.ContactsHandler{Recruiter: recruiterSvc},
	}
}

func composeReferral(p *Platform) *referralhttp.ReferralHandler {
	referralSvc := referral.NewService(p.DB.Queries, p.DB.Queries, referral.NewGitHubCrossReferencer())
	return &referralhttp.ReferralHandler{Referral: referralSvc}
}

func composeOutreach(p *Platform, recruiterSvc *recruiter.Service, companyIntelSvc *companyintel.Service, defaultRouter *llm.Router) *outreachhttp.OutreachHandler {
	outreachSvc := outreach.NewService(recruiterSvc, companyIntelSvc, defaultRouter, "")
	return &outreachhttp.OutreachHandler{Outreach: outreachSvc}
}

func composeInterviewPrep(p *Platform, profileSvc *profile.Service, companyIntelSvc *companyintel.Service) *interviewprephttp.InterviewPrepHandler {
	stories := func(ctx context.Context) ([]keyword.StarStory, error) {
		prof, err := profileSvc.GetDefault(ctx)
		if err != nil {
			return nil, err
		}
		rows, err := p.DB.Queries.ListStarStoriesByProfile(ctx, prof.ID)
		if err != nil {
			return nil, err
		}
		out := make([]keyword.StarStory, len(rows))
		for i, r := range rows {
			var skills []string
			_ = dbutil.UnmarshalJSONB(r.Skills, &skills)
			var categories []keyword.StoryCategory
			_ = dbutil.UnmarshalJSONB(r.Categories, &categories)
			out[i] = keyword.StarStory{
				ID:         dbutil.UUIDString(r.ID),
				ProfileID:  dbutil.UUIDString(r.ProfileId),
				Title:      r.Title,
				Situation:  r.Situation,
				Task:       r.Task,
				Action:     r.Action,
				Result:     r.Result,
				Skills:     skills,
				Categories: categories,
			}
		}
		return out, nil
	}
	prepSvc := interviewprep.NewService(p.DB.Queries, p.DB.Queries, stories, companyIntelSvc)
	return &interviewprephttp.InterviewPrepHandler{InterviewPrep: prepSvc}
}

type redisPinger struct{ client redis.UniversalClient }

func (p redisPinger) Ping(ctx context.Context) error {
	return p.client.Ping(ctx).Err()
}

func composeHealth(p *Platform) *health.HealthHandler {
	redisClient, _ := p.RedisOpt.MakeRedisClient().(redis.UniversalClient)
	return &health.HealthHandler{
		Postgres: p.DB.Pool,
		Redis:    redisPinger{client: redisClient},
		Pool:     p.DB,
	}
}

type hostRetrievalAdapter struct{ svc retrieval.Service }

func (a hostRetrievalAdapter) HostStatus(ctx context.Context, host string) (dto.HostRetrievalStatusDto, error) {
	st, err := a.svc.HostStatus(ctx, host)
	if err != nil {
		return dto.HostRetrievalStatusDto{}, err
	}
	var lastBlockReason *string
	if st.LastBlockReason != "" {
		lastBlockReason = &st.LastBlockReason
	}
	return dto.HostRetrievalStatusDto{
		Host:              st.Host,
		IdentityVersion:   st.IdentityVersion,
		CurrentRung:       st.CurrentRung,
		LastBlockAt:       st.LastBlockAt,
		LastBlockReason:   lastBlockReason,
		CoolingOffUntil:   st.CoolingOffUntil,
		CrawlDelaySeconds: st.CrawlDelaySeconds,
		Pacing: dto.HostPacingDto{
			RequestsPerSecond: st.Pacing.RequestsPerSecond,
			IntervalSeconds:   st.Pacing.IntervalSeconds,
			Source:            st.Pacing.Source,
		},
	}, nil
}

func (a hostRetrievalAdapter) ClearRungPreference(ctx context.Context, host string) error {
	return a.svc.ClearRungPreference(ctx, host)
}

func (a hostRetrievalAdapter) ClearCookies(ctx context.Context, host string) error {
	return a.svc.ClearCookies(ctx, host)
}

func (a hostRetrievalAdapter) OverrideCoolingOff(ctx context.Context, host string) (time.Duration, error) {
	return a.svc.OverrideCoolingOff(ctx, host)
}

func composeRetrieval(p *Platform) (retrieval.Service, error) {
	identity, err := retrieval.NewBrowserIdentity(p.Config.BrowserIdentityVersion)
	if err != nil {
		return nil, err
	}
	store := retrieval.NewStateStore(p.DB.Queries, p.Config.ConfigEncryptionKey)
	return retrieval.NewServiceImpl(identity, store, p.Config)
}

func composeHosts(retrievalSvc retrieval.Service) *jobsourceshttp.HostsHandler {
	return &jobsourceshttp.HostsHandler{Retrieval: hostRetrievalAdapter{svc: retrievalSvc}}
}

func queueClassResolvers(policies []queue.TaskPolicy, llmH *llmHandles) map[string]queue.ClassResolver {
	resolvers := make(map[string]queue.ClassResolver, len(policies))
	for _, p := range policies {
		switch p.LLMTaskKey {
		case "match":
			resolvers[p.Queue] = llmH.MatchRouter
		case "generation":
			resolvers[p.Queue] = llmH.GenerationRouter
		case "default":
			resolvers[p.Queue] = llmH.DefaultRouter
		case "ghost":
			resolvers[p.Queue] = llmH.GhostRouter
		}
	}
	return resolvers
}

func buildContexts(ctx context.Context, p *Platform) (*App, error) {
	sources := composeJobSources(p)
	ingestionH := composeIngestion(p, sources)

	llmH, err := composeLLM(p)
	if err != nil {
		return nil, err
	}
	profileH := composeProfile(p, llmH.Ollama)

	matchingH, err := composeMatching(ctx, p, profileH.Profile, llmH.MatchRouter)
	if err != nil {
		return nil, err
	}
	ghostH := composeGhostJob(p, llmH.GhostRouter)

	generationH, err := composeGeneration(ctx, p, profileH.Profile, generation.Routers{
		Analyze: llmH.GenerationAnalyzeRouter,
		Select:  llmH.GenerationSelectRouter,
		Premium: llmH.GenerationPremiumRouter,
		Summary: llmH.GenerationSummaryRouter,
		// 034: one router per hosted summary option. The `standard` option is
		// served by Summary above — it is the same task key the pipeline used
		// before the feature existed, which is what makes an untouched selector
		// byte-identical to today. The self-hosted option is a router with no
		// gateway, which is the pre-split behaviour.
		SummaryByOption: llmH.SummaryOptionRouters,
		Cover:           llmH.GenerationRouter,
	})
	if err != nil {
		return nil, err
	}
	retrievalSvc, err := composeRetrieval(p)
	if err != nil {
		return nil, err
	}

	enrichHandler := composeEnrichment(p, sources, retrievalSvc)
	ingestionH.Sources.Enrichment = enrichHandler

	jobsHandler := &jobshttp.JobsHandler{Jobs: matchingH.Jobs, Generation: generationH.Generation, Enrichment: enrichHandler}

	salaryH := composeSalary(ctx, p, llmH.DefaultRouter)

	keywordH := composeKeyword(p, llmH.RephraseRouter, profileH.Profile)
	companyIntelH := composeCompanyIntel(p)
	recruiterH := composeRecruiter(p, llmH.DefaultRouter)
	interviewPrepH := composeInterviewPrep(p, profileH.Profile, companyIntelH.Service)
	hostsH := composeHosts(retrievalSvc)

	return &App{
		Sources:       ingestionH.Sources,
		Roster:        &jobsourceshttp.RosterHandler{Roster: roster.NewView(sources.Roster)},
		Searches:      ingestionH.Searches,
		Documents:     generationH.Documents,
		Profiles:      profileH.Handler,
		Jobs:          jobsHandler,
		Applications:  composeApplications(p),
		Subs:          composeSubscriptions(p, sources.Sources, ingestionH.Ingestion),
		ManualAdd:     composeManualAdd(p, sources, matchingH.Jobs),
		Activity:      activityhttp.NewActivityHandler(p.DB.Queries, p.AsynqClient, p.AsynqInspector, p.Policies, queueClassResolvers(p.Policies, llmH)),
		Keyword:       keywordH.Handler,
		PostAge:       composePostAge(p),
		Notification:  composeNotifications(p),
		Companies:     companyIntelH.Handler,
		GhostJob:      ghostH.HTTPHandler,
		Coach:         composeCoach(p, keywordH.RephraseModel, profileH.Profile),
		Contacts:      recruiterH.Handler,
		Referral:      composeReferral(p),
		Outreach:      composeOutreach(p, recruiterH.Service, companyIntelH.Service, llmH.DefaultRouter),
		AiFeatures:    matchingH.AiFeatureHandler,
		ResumeShape:   generationH.ResumeShape,
		SummaryModel:  generationH.SummaryModel,
		InterviewPrep: interviewPrepH,
		Health:        composeHealth(p),
		Hosts:         hostsH,

		Ingestion:  ingestionH.Handler,
		Matching:   matchingH.Handler,
		Generation: generationH.Handler,
		Enrichment: enrichHandler,
		Salary:     salaryH.Worker,
		Ghost:      ghostH.Worker,

		MatchRouter:      llmH.MatchRouter,
		GenerationRouter: llmH.GenerationRouter,
		DefaultRouter:    llmH.DefaultRouter,
		GhostRouter:      llmH.GhostRouter,

		Scheduler: ingestionH.Scheduler,
	}, nil
}

// summaryOptionRouters builds one provider per 034 summary option, from the
// catalogue rather than from a list repeated here — an option added in
// internal/generation/domain wires itself.
//
// The self-hosted option deliberately gets a router with **no gateway**: that
// is what "runs entirely on your own machine" means, and it is the same shape
// the whole application takes when no gateway is configured at all.
func summaryOptionRouters(gateway, local llm.Provider, localModel string) map[string]llm.Provider {
	out := map[string]llm.Provider{}
	for _, o := range generation.SummaryOptions() {
		if o.SelfHosted() {
			out[o.ID] = llm.NewRouter("", nil, local, localModel)
			continue
		}
		out[o.ID] = llm.NewRouter(o.TaskKey, gateway, local, localModel)
	}
	return out
}
