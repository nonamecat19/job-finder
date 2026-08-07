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
