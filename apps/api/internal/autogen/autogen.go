// Package autogen is the facade for the auto-generate-on-high-match-score
// setting: the Repository outbound port and State type live in domain/,
// the in-memory-cached Service orchestration lives in application/. This
// file re-exports the shape callers already depend on (compose.go,
// httpapi/autogenerate.go) so relocating the package required no changes
// at call sites beyond the import path.
package autogen

import (
	"github.com/job-finder/api/internal/autogen/application"
	"github.com/job-finder/api/internal/autogen/domain"
)

type (
	Repository = domain.Repository
	State      = domain.State

	Service = application.Service
)

var NewService = application.NewService
