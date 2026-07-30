package ghostjob

import (
	"github.com/job-finder/api/internal/ghostjob/application"
	"github.com/job-finder/api/internal/ghostjob/domain"
	"github.com/job-finder/api/internal/ghostjob/interfaces/worker"
)

type (
	Service        = application.Service
	Handler        = worker.Handler
	Repository     = domain.Repository
	GhostJobResult = domain.GhostJobResult
	GhostSignals   = domain.GhostSignals
)

var (
	NewService         = application.NewService
	NewHandler         = worker.NewHandler
	ErrDeclinedToScore = application.ErrDeclinedToScore
	Kind               = application.Kind
)
