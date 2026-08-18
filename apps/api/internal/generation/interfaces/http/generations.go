package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

type WorkspaceGenerator interface {
	StartGenerationRun(ctx context.Context, req dto.StartGenerationRequestDto) (runID, activityID string, err error)
	GetGenerationWorkspace(ctx context.Context, runID string) (dto.GenerationRunDto, error)
	ListGenerationRuns(ctx context.Context, profileID string, jobID *string, limit int) ([]dto.GenerationRunDto, error)
	DeleteGenerationRun(ctx context.Context, runID string) error

	PatchGenerationItem(ctx context.Context, runID, itemID string, req dto.PatchGenerationItemRequestDto) (dto.GenerationItemDto, error)
	ReorderSection(ctx context.Context, runID, sectionID string, itemIDs []string) (dto.GenerationSectionDto, error)

	SetSectionEnabled(ctx context.Context, runID, sectionID string, enabled bool) (dto.GenerationSectionDto, error)

	ExportGenerationRun(ctx context.Context, runID string) (dto.GenerationExportDto, error)
	GetGenerationExport(ctx context.Context, runID string) (dto.GenerationExportDto, error)

	RerunGenerationRun(ctx context.Context, runID string, req dto.RerunGenerationRequestDto) (runID2, activityID string, err error)

	RewriteGenerationItem(ctx context.Context, runID, itemID string) (dto.GenerationRewriteResponseDto, error)

	PreviewDocument(ctx context.Context, runID string) (dto.PreviewDocumentDto, error)
}

type GenerationsHandler struct {
	Workspace WorkspaceGenerator
}

func (h *GenerationsHandler) Mount(r chi.Router) {
	r.Post("/generations", h.start)
	r.Get("/generations", h.list)
	r.Get("/generations/{runId}", h.get)
	r.Delete("/generations/{runId}", h.remove)
	r.Patch("/generations/{runId}/items/{itemId}", h.patchItem)
	r.Post("/generations/{runId}/items/{itemId}/rewrite", h.rewriteItem)
	r.Patch("/generations/{runId}/sections/{sectionId}/order", h.reorderSection)
	r.Patch("/generations/{runId}/sections/{sectionId}", h.patchSection)
	r.Post("/generations/{runId}/rerun", h.rerun)
	r.Post("/generations/{runId}/export", h.export)
	r.Get("/generations/{runId}/export", h.exportStatus)
	r.Get("/generations/{runId}/preview-document", h.previewDocument)
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

func (h *GenerationsHandler) patchItem(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	itemID := chi.URLParam(r, "itemId")
	var body dto.PatchGenerationItemRequestDto
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.Workspace.PatchGenerationItem(r.Context(), runID, itemID, body)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) rewriteItem(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	itemID := chi.URLParam(r, "itemId")
	out, err := h.Workspace.RewriteGenerationItem(r.Context(), runID, itemID)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) export(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	out, err := h.Workspace.ExportGenerationRun(r.Context(), runID)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	if out.Status == "blocked" {
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func (h *GenerationsHandler) previewDocument(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	out, err := h.Workspace.PreviewDocument(r.Context(), runID)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) exportStatus(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	out, err := h.Workspace.GetGenerationExport(r.Context(), runID)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) rerun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	var body dto.RerunGenerationRequestDto
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	newRunID, activityID, err := h.Workspace.RerunGenerationRun(r.Context(), runID, body)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"runId": newRunID, "activityId": activityID})
}

func (h *GenerationsHandler) patchSection(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	sectionID := chi.URLParam(r, "sectionId")
	var body dto.PatchGenerationSectionRequestDto
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Enabled == nil {
		httpx.WriteError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	out, err := h.Workspace.SetSectionEnabled(r.Context(), runID, sectionID, *body.Enabled)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *GenerationsHandler) reorderSection(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	sectionID := chi.URLParam(r, "sectionId")
	var body dto.ReorderSectionItemsRequestDto
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.Workspace.ReorderSection(r.Context(), runID, sectionID, body.ItemIDs)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
