package applications

import "github.com/job-finder/api/internal/applications/application"

type (
	Service     = application.Service
	UpdateInput = application.UpdateInput
)

var (
	NewService  = application.NewService
	ErrNotFound = application.ErrNotFound
)
