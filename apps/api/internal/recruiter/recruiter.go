// Package recruiter is the facade for the Recruiter Resolution bounded
// context (spec 007): the ResolvedContact model and outbound ports live in
// domain/, orchestration across the posting/company-page/linkedin sources
// in application/. This file re-exports the shape callers already depend
// on (compose.go) so relocating the package required no changes at call
// sites beyond the import path.
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
