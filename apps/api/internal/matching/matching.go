package matching

import (
	"github.com/job-finder/api/internal/matching/application"
	"github.com/job-finder/api/internal/matching/domain"
	"github.com/job-finder/api/internal/matching/interfaces/worker"
)

type (
	Repository = domain.Repository
	FitResult  = domain.FitResult

	Service = application.Service

	Handler          = worker.Handler
	AutoGenerateGate = worker.AutoGenerateGate
	Generator        = worker.Generator

	Publisher     = application.Publisher
	MatchSnapshot = application.MatchSnapshot
)

const Kind = application.Kind

var (
	ErrNoProfileConfig = application.ErrNoProfileConfig

	NewService       = application.NewService
	NewHandler       = worker.NewHandler
	NewResultHandler = application.NewResultHandler
)
