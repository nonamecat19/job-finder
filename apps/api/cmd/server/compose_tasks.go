package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/job-finder/api/internal/coach"
	"github.com/job-finder/api/internal/companyintel"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/interviewprep"
	"github.com/job-finder/api/internal/keyword"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/profile"
	"github.com/job-finder/api/internal/salary"
)

func composeEnrichment(p *Platform, sources *sourcesHandles) *enrichment.Handler {
	cfg := p.Config
	enrichDelay := time.Duration(cfg.DjinniDetailDelayMs) * time.Millisecond
	enrichDelays := map[string]time.Duration{
		"workua": time.Duration(cfg.WorkUaDetailDelayMs) * time.Millisecond,
	}
	return enrichment.NewHandler(p.DB.Queries, sources.Sources, sources.Djinni, sources.Dou, sources.Workua, sources.Indeed, sources.RemoteOK, sources.Glassdoor, sources.JobLeads, sources.Wellfound, sources.Jobgether, p.AsynqClient, enrichDelay, enrichDelays)
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
	Handler       *httpapi.KeywordHandler
	RephraseModel *keyword.ProviderRephraseModel
}

func composeKeyword(p *Platform, rephraseRouter *llm.Router, profileSvc *profile.Service) *keywordHandles {
	rephraseModel := keyword.NewProviderRephraseModel(rephraseRouter, "")
	cached := keyword.NewCachedRephraser(
		keyword.NewSuggester(rephraseModel),
		time.Duration(p.Config.KeywordRephraseCacheTTLSec)*time.Second,
	)
	diffSvc := keyword.NewDiffService(p.DB.Queries).WithRephraser(cached, profileSvc)
	return &keywordHandles{
		Handler:       &httpapi.KeywordHandler{Diff: diffSvc},
		RephraseModel: rephraseModel,
	}
}

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

func composeInterviewPrep(p *Platform, profileSvc *profile.Service, companyIntelSvc *companyintel.Service) *httpapi.InterviewPrepHandler {
	prepSvc := interviewprep.NewService(p.DB.Queries, p.DB.Queries, func(ctx context.Context) ([]keyword.StarStory, error) {
		prof, err := profileSvc.GetDefault(ctx)
		if err != nil {
			return nil, nil
		}
		uid, _ := dbutil.ParseUUID(prof.ID)
		rows, err := p.DB.Queries.ListStarStoriesByProfile(ctx, uid)
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
