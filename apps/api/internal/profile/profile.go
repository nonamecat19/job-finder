// Package profile is the facade for the Profile bounded context: the
// Repository outbound port and the Entry grounding-unit type live in
// domain/, all CRUD/embedding/rendercv orchestration (Service) lives in
// application/. This file re-exports the shape callers already depend on
// (compose.go, matching, generation, keyword, coach, extauth, httpapi) so
// relocating the package required no changes at call sites beyond the
// import path.
package profile

import (
	"github.com/job-finder/api/internal/profile/application"
	"github.com/job-finder/api/internal/profile/domain"
)

type (
	Repository = domain.Repository
	Entry      = domain.Entry

	Service     = application.Service
	UpdateInput = application.UpdateInput
)

var NewService = application.NewService
