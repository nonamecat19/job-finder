package main

import (
	"time"

	"github.com/job-finder/api/internal/companyintel"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/outreach"
	"github.com/job-finder/api/internal/recruiter"
	"github.com/job-finder/api/internal/referral"
)

type companyIntelHandles struct {
	Service *companyintel.Service
	Handler *httpapi.CompaniesHandler
}

func composeCompanyIntel(p *Platform) *companyIntelHandles {
	registry := companyintel.NewRegistry(
		companyintel.CrunchbaseScraper{Scraping: p.Scraping},
		companyintel.LayoffsScraper{Scraping: p.Scraping},
		companyintel.GlassdoorScraper{Scraping: p.Scraping},
		companyintel.HeadcountScraper{Scraping: p.Scraping},
		companyintel.TechStackScraper{Scraping: p.Scraping},
	)
	svc := companyintel.NewService(p.DB.Queries, registry, 2*time.Second)
	return &companyIntelHandles{
		Service: svc,
		Handler: &httpapi.CompaniesHandler{CompanyIntel: svc},
	}
}

type recruiterHandles struct {
	Service *recruiter.Service
	Handler *httpapi.ContactsHandler
}

func composeRecruiter(p *Platform, defaultRouter *llm.Router) *recruiterHandles {
	svc := recruiter.NewService(p.DB.Queries, defaultRouter, "", p.Scraping, p.Config.LinkedInScrapeEnabled)
	return &recruiterHandles{
		Service: svc,
		Handler: &httpapi.ContactsHandler{Recruiter: svc},
	}
}

func composeReferral(p *Platform) *httpapi.ReferralHandler {
	svc := referral.NewService(p.DB.Queries, p.DB.Queries, referral.NewGitHubCrossReferencer())
	return &httpapi.ReferralHandler{Referral: svc}
}

func composeOutreach(p *Platform, recruiterSvc *recruiter.Service, companyIntelSvc *companyintel.Service, defaultRouter *llm.Router) *httpapi.OutreachHandler {
	svc := outreach.NewService(recruiterSvc, companyIntelSvc, defaultRouter, "")
	return &httpapi.OutreachHandler{Outreach: svc}
}
