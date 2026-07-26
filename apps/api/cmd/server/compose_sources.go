package main

import (
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/jobsources/roster"
)

func composeJobSources(p *Platform) *sourcesHandles {
	djinniAdapter := adapters.DjinniAdapter{Scraping: p.Scraping, Session: p.DjinniSession}
	douAdapter := adapters.DouAdapter{Scraping: p.Scraping}
	workuaAdapter := adapters.WorkUaAdapter{Scraping: p.Scraping}
	indeedAdapter := adapters.IndeedAdapter{Scraping: p.Scraping}
	remoteokAdapter := adapters.RemoteOKAdapter{Scraping: p.Scraping}
	glassdoorAdapter := adapters.GlassdoorAdapter{Scraping: p.Scraping}
	jobleadsAdapter := adapters.JobLeadsAdapter{Scraping: p.Scraping, Session: p.JobLeadsSession}
	wellfoundAdapter := adapters.WellfoundAdapter{Scraping: p.Scraping}
	himalayasAdapter := adapters.HimalayasAdapter{Scraping: p.Scraping}
	jobgetherAdapter := adapters.JobgetherAdapter{Scraping: p.Scraping}

	gh, lv, as, wk, sr, boardCheckers := adapters.NewBoardAdapters()
	rosterSvc := roster.NewService(p.DB.Queries, boardCheckers)
	gh.Roster, lv.Roster, as.Roster, wk.Roster, sr.Roster = rosterSvc, rosterSvc, rosterSvc, rosterSvc, rosterSvc

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
		wellfoundAdapter,
		himalayasAdapter,
		jobgetherAdapter,
		gh, lv, as, wk, sr,
	)
	sourcesSvc := jobsources.NewService(p.DB.Queries, registry, p.Config.ConfigEncryptionKey)
	p.DjinniSession.Sources = sourcesSvc
	p.JobLeadsSession.Sources = sourcesSvc

	return &sourcesHandles{
		Registry:  registry,
		Sources:   sourcesSvc,
		Roster:    rosterSvc,
		Djinni:    djinniAdapter,
		Dou:       douAdapter,
		Workua:    workuaAdapter,
		Indeed:    indeedAdapter,
		RemoteOK:  remoteokAdapter,
		Glassdoor: glassdoorAdapter,
		JobLeads:  jobleadsAdapter,
		Wellfound: wellfoundAdapter,
		Himalayas: himalayasAdapter,
		Jobgether: jobgetherAdapter,
	}
}

func composeIngestion(p *Platform, sources *sourcesHandles) *ingestionHandles {
	ingestionSvc := ingestion.NewService(p.DB.Queries, sources.Registry, sources.Sources, p.AsynqClient)
	handler := ingestion.NewHandler(p.DB.Queries, sources.Registry, sources.Sources, p.AsynqClient)
	scheduler := ingestion.NewScheduler(p.DB.Queries, ingestionSvc)
	return &ingestionHandles{
		Ingestion: ingestionSvc,
		Handler:   handler,
		Scheduler: scheduler,
		Sources:   &httpapi.SourcesHandler{Sources: sources.Sources, Ingestion: ingestionSvc},
		Searches:  &httpapi.SearchesHandler{Ingestion: ingestionSvc},
	}
}
