package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

// WorkspaceGenerator is the 042 resume generation workspace's application
// surface (contracts/rest-api.md). Phase 2 (Foundational) wires start / get /
// list / delete; item/section mutation, rerun and export are later phases.
type WorkspaceGenerator interface {
	StartGenerationRun(ctx context.Context, req dto.StartGenerationRequestDto) (runID, activityID string, err error)
	GetGenerationWorkspace(ctx context.Context, runID string) (dto.GenerationRunDto, error)
	ListGenerationRuns(ctx context.Context, profileID string, jobID *string, limit int) ([]dto.GenerationRunDto, error)
	DeleteGenerationRun(ctx context.Context, runID string) error
}

type GenerationsHandler struct {
	Workspace WorkspaceGenerator
}

func (h *GenerationsHandler) Mount(r chi.Router) {
	r.Post("/generations", h.start)
	r.Get("/generations", h.list)
	r.Get("/generations/{runId}", h.get)
	r.Delete("/generations/{runId}", h.remove)
}

func (h *GenerationsHandler) start(w http.ResponseWriter, r *http.Request) {
	var body dto.StartGenerationRequestDto
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	runID, activityID, err := h.Workspace.StartGenerationRun(r.Context(), body)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"runId": runID, "activityId": activityID})
}

func (h *GenerationsHandler) get(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	out, err := h.Workspace.GetGenerationWorkspace(r.Context(), runID)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) list(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profileId")
	var jobID *string
	if v := r.URL.Query().Get("jobId"); v != "" {
		jobID = &v
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	out, err := h.Workspace.ListGenerationRuns(r.Context(), profileID, jobID, limit)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) remove(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	if err := h.Workspace.DeleteGenerationRun(r.Context(), runID); err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
