// Package llmsettings is the facade for the LLM task-routing settings
// bounded context (001-cerebras-model-toggle): TaskKeys, TaskUpdate,
// State, sentinel errors, and the Repository outbound port live in
// domain/; Service (which owns the llm.SnapshotHolder every llm.Router
// reads from) lives in application/. This file re-exports the shape
// callers already depend on (compose.go, httpapi/llm_settings.go) so
// relocating the package required no changes at call sites beyond the
// import path.
package llmsettings

import (
	"github.com/job-finder/api/internal/llmsettings/application"
	"github.com/job-finder/api/internal/llmsettings/domain"
)

type (
	Repository = domain.Repository
	TaskUpdate = domain.TaskUpdate
	State      = domain.State

	Service = application.Service
)

var (
	TaskKeys = domain.TaskKeys

	ErrUnknownTaskKey  = domain.ErrUnknownTaskKey
	ErrInvalidProvider = domain.ErrInvalidProvider
	ErrInvalidModel    = domain.ErrInvalidModel

	NewService = application.NewService
)
