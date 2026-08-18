package ghostjob

import (
	"github.com/job-finder/api/internal/ghostjob/application"
	"github.com/job-finder/api/internal/ghostjob/domain"
	"github.com/job-finder/api/internal/ghostjob/interfaces/worker"
)

type (
	Service          = application.Service
	Handler          = worker.Handler
	Repository       = domain.Repository
	GhostJobResult   = domain.GhostJobResult
	GhostSignals     = domain.GhostSignals
	SnapshotEnqueuer = application.SnapshotEnqueuer
	TxRunner         = application.TxRunner
)

var (
	NewService         = application.NewService
	NewHandler         = worker.NewHandler
	NewResultHandler   = application.NewResultHandler
	ErrDeclinedToScore = application.ErrDeclinedToScore
	Kind               = application.Kind
)
