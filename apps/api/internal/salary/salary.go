package salary

import (
	"github.com/job-finder/api/internal/salary/application"
	"github.com/job-finder/api/internal/salary/domain"
	salaryworker "github.com/job-finder/api/internal/salary/interfaces/worker"
)

type (
	Service          = application.Service
	LevelsFyiLoader  = application.LevelsFyiLoader
	Handler          = salaryworker.Handler
	Repository       = application.Repository
	SalaryBand       = domain.SalaryBand
	SalarySource     = domain.SalarySource
	BlendedBand      = domain.BlendedBand
	SnapshotEnqueuer = application.SnapshotEnqueuer
	TxRunner         = application.TxRunner
)

var (
	NewService         = application.NewService
	NewLevelsFyiLoader = application.NewLevelsFyiLoader
	NewHandler         = salaryworker.NewHandler
	NewResultHandler   = application.NewResultHandler
	ParseSalaryRaw     = domain.ParseSalaryRaw
	Kind               = application.Kind

	SourceLLM           = domain.SourceLLM
	SourceLevelsFyi     = domain.SourceLevelsFyi
	SourceIngestedCache = domain.SourceIngestedCache
	SourceBlended       = domain.SourceBlended
)
