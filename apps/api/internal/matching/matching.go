// Package matching is the facade for the Matching bounded context: the
// Repository persistence port, the FitResult structured-LLM-output schema,
// and the ErrNoProfileConfig sentinel live in domain/, the MatchJob
// use-case (Service) lives in application/, and the asynq "match" task
// handler lives in interfaces/worker/. This file re-exports the shape
// callers already depend on (cmd/server/compose.go) so relocating the
// package required no changes at call sites beyond the import path.
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
)

var (
	ErrNoProfileConfig = application.ErrNoProfileConfig

	NewService = application.NewService
	NewHandler = worker.NewHandler
)
