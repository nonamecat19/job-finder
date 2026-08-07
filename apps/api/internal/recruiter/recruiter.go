package recruiter

import (
	"github.com/job-finder/api/internal/recruiter/application"
	"github.com/job-finder/api/internal/recruiter/domain"
)

type (
	ResolvedContact = domain.ResolvedContact
	Repository      = domain.Repository
	ScrapingService = domain.ScrapingService

	Service = application.Service
)

var NewService = application.NewService

const (
	SourcePosting     = domain.SourcePosting
	SourceCompanyPage = domain.SourceCompanyPage
	SourceLinkedIn    = domain.SourceLinkedIn
)
