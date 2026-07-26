package main

import (
	"context"
	"log/slog"

	"github.com/job-finder/api/internal/aifeature"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/jobs"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/storage"
)

type profileHandles struct {
	Profile *profile.Service
	Handler *httpapi.ProfilesHandler
}

func composeProfile(p *Platform, ollama *llm.OllamaProvider) *profileHandles {
	svc := profile.NewService(profile.NewSqlcRepository(p.DB.Queries), ollama, p.Config.EmbedModel, p.Config.RendercvBin)
	return &profileHandles{
		Profile: svc,
		Handler: &httpapi.ProfilesHandler{Profiles: svc},
	}
}

type matchingHandles struct {
	Jobs             *jobs.Service
	Notifier         *notifier.Service
	AiFeatures       *aifeature.Service
	AiFeatureHandler *httpapi.AiFeatureHandler
	Handler          *matching.Handler
}

func composeMatching(ctx context.Context, p *Platform, profileSvc *profile.Service, matchRouter *llm.Router) (*matchingHandles, error) {
	matchingSvc := matching.NewService(matching.NewSqlcRepository(p.DB.Queries), profileSvc, matchRouter, p.Config.MatchSimilarityThreshold, "")
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

func composeGhostJob(p *Platform, ghostRouter *llm.Router) *ghostHandles {
	svc := ghostjob.NewService(p.DB.Queries, ghostRouter, "")
	return &ghostHandles{
		Worker:      ghostjob.NewHandler(svc, p.DB.Queries),
		HTTPHandler: &httpapi.GhostJobHandler{Ghost: svc},
	}
}

type generationHandles struct {
	Generation *generation.Service
	Handler    *generation.Handler
	Documents  *httpapi.DocumentsHandler
}

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
	genSvc := generation.NewService(p.DB.Queries, profileSvc, htmlRenderer, rendercvRenderer, generationRouter, "", cfg.ResumeMasterPath, cfg.ResumeGroundingLvl)
	return &generationHandles{
		Generation: genSvc,
		Handler:    generation.NewHandler(genSvc),
		Documents:  &httpapi.DocumentsHandler{Generation: genSvc},
	}, nil
}
