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
	ParseGroundingLevel     = domain.ParseGroundingLevel
	CvSections              = domain.CvSections
	AsSliceOfMaps           = domain.AsSliceOfMaps
	StringField             = domain.StringField
	StringSliceField        = domain.StringSliceField
	MasterFromProfile       = domain.MasterFromProfile
	RendercvToText          = domain.RendercvToText
	ParseRendercv           = domain.ParseRendercv
	PrepareMasterForMarshal = domain.PrepareMasterForMarshal

	sectionOrderKey = domain.SectionOrderKey

	deepCloneYAML = domain.DeepCloneYAML

	sortByDefaultSectionOrder = domain.SortByDefaultSectionOrder

	NewService          = application.NewService
	GetDefaultMasterID  = application.GetDefaultMasterID
	NewHtmlPdfRenderer  = infrastructure.NewHtmlPdfRenderer
	NewRenderCvRenderer = infrastructure.NewRenderCvRenderer
	NewHandler          = worker.NewHandler
)
