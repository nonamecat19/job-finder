package companyintel

import (
	"github.com/job-finder/api/internal/companyintel/application"
	cia "github.com/job-finder/api/internal/companyintel/infrastructure/adapters"
	"github.com/job-finder/api/internal/companyintel/domain"
)

type (
	Service           = application.Service
	Registry          = domain.Registry
	CrunchbaseScraper = cia.CrunchbaseScraper
	LayoffsScraper    = cia.LayoffsScraper
	GlassdoorScraper  = cia.GlassdoorScraper
	HeadcountScraper  = cia.HeadcountScraper
	TechStackScraper  = cia.TechStackScraper
)

var (
	NewService  = application.NewService
	NewRegistry = domain.NewRegistry
	ErrNoCompany = application.ErrNoCompany
)
