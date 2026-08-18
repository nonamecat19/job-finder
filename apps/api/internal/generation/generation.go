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
	SummaryOption   = domain.SummaryOption

	Routers = application.GenerationRouters
	Service = application.Service

	HtmlPdfRenderer  = infrastructure.HtmlPdfRenderer
	RenderCvRenderer = infrastructure.RenderCvRenderer

	Handler = worker.Handler

	SnapshotEnqueuer     = application.SnapshotEnqueuer
	TxRunner             = application.TxRunner
	ShapeProvider        = application.ShapeProvider
	SummaryModelProvider = application.SummaryModelProvider
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

	// 034 summary-model choice.
	SummaryOptions       = domain.SummaryOptions
	DefaultSummaryOption = domain.DefaultSummaryOption
	LookupSummaryOption  = domain.LookupSummaryOption
	WithSummaryOption    = application.WithSummaryOption

	sectionOrderKey = domain.SectionOrderKey

	deepCloneYAML = domain.DeepCloneYAML

	sortByDefaultSectionOrder = domain.SortByDefaultSectionOrder

	NewService          = application.NewService
	GetDefaultMasterID  = application.GetDefaultMasterID
	NewHtmlPdfRenderer  = infrastructure.NewHtmlPdfRenderer
	NewRenderCvRenderer = infrastructure.NewRenderCvRenderer
	NewHandler          = worker.NewHandler
	NewResultHandler    = application.NewResultHandler
	Kind                = application.Kind
)
