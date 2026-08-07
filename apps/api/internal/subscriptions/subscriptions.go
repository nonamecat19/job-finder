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
