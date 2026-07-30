// Package generation is the facade for the Document Generation bounded
// context: the pure RendercvMaster/grounding model lives in domain/, the
// tailoring/rendering orchestration in application/, the HTML+chromedp and
// RenderCV-CLI adapters in infrastructure/, and the asynq task handler in
// interfaces/worker/. This file re-exports the shape callers already depend
// on (matching, profile, httpapi, cmd/server) so relocating the package
// required no changes at call sites beyond the import path.
package generation

import (
	"github.com/job-finder/api/internal/generation/application"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/generation/infrastructure"
	"github.com/job-finder/api/internal/generation/interfaces/worker"
)

type (
	RendercvMaster  = domain.RendercvMaster
	GroundingLevel  = domain.GroundingLevel
	VacancyHints    = domain.VacancyHints
	VacancyAnalysis = domain.VacancyAnalysis

	AdHocInput = application.AdHocInput
	Service    = application.Service

	HtmlPdfRenderer  = infrastructure.HtmlPdfRenderer
	RenderCvRenderer = infrastructure.RenderCvRenderer

	Handler = worker.Handler
)

var (
	ParseGroundingLevel = domain.ParseGroundingLevel
	CvSections          = domain.CvSections
	AsSliceOfMaps       = domain.AsSliceOfMaps
	StringField         = domain.StringField
	StringSliceField    = domain.StringSliceField
	MasterFromProfile   = domain.MasterFromProfile
	RendercvToText      = domain.RendercvToText
	ParseRendercv       = domain.ParseRendercv

	NewService          = application.NewService
	GetDefaultMasterID  = application.GetDefaultMasterID
	NewHtmlPdfRenderer  = infrastructure.NewHtmlPdfRenderer
	NewRenderCvRenderer = infrastructure.NewRenderCvRenderer
	NewHandler          = worker.NewHandler
)
