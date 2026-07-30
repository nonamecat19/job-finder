// Package subscriptions is the facade for the URL-subscriptions bounded
// context: the Repository/SourceEnsurer outbound ports live in domain/,
// the CRUD + enable-toggle orchestration in application/. This file
// re-exports the shape callers already depend on (compose.go,
// httpapi/subscriptions.go) so relocating the package required no changes
// at call sites beyond the import path.
package subscriptions

import (
	"github.com/job-finder/api/internal/subscriptions/application"
	"github.com/job-finder/api/internal/subscriptions/domain"
)

type (
	Repository    = domain.Repository
	SourceEnsurer = domain.SourceEnsurer

	Service     = application.Service
	UpdateInput = application.UpdateInput
)

var NewService = application.NewService
