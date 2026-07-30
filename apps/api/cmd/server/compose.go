package main

import (
	"context"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/queue"
)

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

	matchingH, err := composeMatching(ctx, p, profileH.Profile, profileH.Snapshot, llmH.MatchRouter)
	if err != nil {
		return nil, err
	}
	ghostH := composeGhostJob(p, llmH.GhostRouter)

	generationH, err := composeGeneration(ctx, p, profileH.Profile, llmH.GenerationRouter)
	if err != nil {
		return nil, err
	}
	enrichHandler := composeEnrichment(p, sources)

	jobsHandler := &httpapi.JobsHandler{Jobs: matchingH.Jobs, Generation: generationH.Generation, Enrichment: enrichHandler}
	ingestionH.Sources.Enrichment = enrichHandler

	salaryH := composeSalary(ctx, p, llmH.DefaultRouter)

	keywordH := composeKeyword(p, llmH.RephraseRouter, profileH.Profile)
	companyIntelH := composeCompanyIntel(p)
	recruiterH := composeRecruiter(p, llmH.DefaultRouter)

	return &App{
		Sources:       ingestionH.Sources,
		Roster:        &httpapi.RosterHandler{Roster: sources.Roster},
		Searches:      ingestionH.Searches,
		Documents:     generationH.Documents,
		Profiles:      profileH.Handler,
		Jobs:          jobsHandler,
		Applications:  composeApplications(p),
		Subs:          composeSubscriptions(p, sources.Sources, ingestionH.Ingestion),
		Activity: httpapi.NewActivityHandler(p.DB.Queries, p.AsynqClient, p.AsynqInspector, p.Policies, map[string]queue.ClassResolver{
			queue.QueueMatch:       llmH.MatchRouter,
			queue.QueueGenerate:    llmH.GenerationRouter,
			queue.QueueSalaryInfer: llmH.DefaultRouter,
			queue.QueueGhostScore:  llmH.GhostRouter,
		}),
		Keyword:       keywordH.Handler,
		PostAge:       composePostAge(p),
		Notification:  composeNotifications(p),
		Companies:     companyIntelH.Handler,
		GhostJob:      ghostH.HTTPHandler,
		Coach:         composeCoach(p, keywordH.RephraseModel, profileH.Profile),
		Contacts:      recruiterH.Handler,
		Referral:      composeReferral(p),
		Outreach:      composeOutreach(p, recruiterH.Service, companyIntelH.Service, llmH.DefaultRouter),
		LlmSettings:   llmH.SettingsHandler,
		AiFeatures:    matchingH.AiFeatureHandler,
		InterviewPrep: composeInterviewPrep(p, profileH.Profile, companyIntelH.Service),
		Health:        composeHealth(p),
		Hosts:         composeHosts(p),

		Ingestion:  ingestionH.Handler,
		Matching:   matchingH.Handler,
		Generation: generationH.Handler,
		Enrichment: enrichHandler,
		Salary:     salaryH.Worker,
		Ghost:      ghostH.Worker,

		MatchRouter:      llmH.MatchRouter,
		GenerationRouter: llmH.GenerationRouter,
		GhostRouter:      llmH.GhostRouter,
		DefaultRouter:    llmH.DefaultRouter,

		Scheduler: ingestionH.Scheduler,
	}, nil
}
