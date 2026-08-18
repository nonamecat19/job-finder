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
// surface (resume-generation.md § 4.1). Phase 2 (Foundational) wires start / get /
// list / delete; item/section mutation, rerun and export are later phases.
type WorkspaceGenerator interface {
	StartGenerationRun(ctx context.Context, req dto.StartGenerationRequestDto) (runID, activityID string, err error)
	GetGenerationWorkspace(ctx context.Context, runID string) (dto.GenerationRunDto, error)
	ListGenerationRuns(ctx context.Context, profileID string, jobID *string, limit int) ([]dto.GenerationRunDto, error)
	DeleteGenerationRun(ctx context.Context, runID string) error
	// PatchGenerationItem and ReorderSection are Phase 3 (US1): item
	// toggle/edit/reorder and whole-section reorder (resume-generation.md § 4.1).
	PatchGenerationItem(ctx context.Context, runID, itemID string, req dto.PatchGenerationItemRequestDto) (dto.GenerationItemDto, error)
	ReorderSection(ctx context.Context, runID, sectionID string, itemIDs []string) (dto.GenerationSectionDto, error)
	// SetSectionEnabled is the per-run "disable this section" switch.
	SetSectionEnabled(ctx context.Context, runID, sectionID string, enabled bool) (dto.GenerationSectionDto, error)
	// ExportGenerationRun and GetGenerationExport are Phase 7 (US5): the
	// render-once export and its idempotent short-poll.
	ExportGenerationRun(ctx context.Context, runID string) (dto.GenerationExportDto, error)
	GetGenerationExport(ctx context.Context, runID string) (dto.GenerationExportDto, error)
	// RerunGenerationRun is Phase 8 (T076): whole-run or named-section rerun,
	// in place on the same run id.
	RerunGenerationRun(ctx context.Context, runID string, req dto.RerunGenerationRequestDto) (runID2, activityID string, err error)
	// RewriteGenerationItem returns grounded alternate phrasings of one
	// AI-suggested achievement bullet. Never persists anything — applying a
	// variant goes through PatchGenerationItem.
	RewriteGenerationItem(ctx context.Context, runID, itemID string) (dto.GenerationRewriteResponseDto, error)
	// PreviewDocument is 046: the run's current selection assembled into
	// RenderCV YAML, without rendering it — the source the dashboard's
	// in-browser WASM pipeline turns into a live PDF preview.
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

// export is `POST /v1/generations/{runId}/export` (rest-api.md): `200` with
// the overflow report when the selection does not fit the page budget, `202`
// otherwise — the client polls `GET …/export` for the document id either way.
// A `409` comes from the service: the run is running, or nothing is selected.
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

// previewDocument is `GET /v1/generations/{runId}/preview-document` (046): a
// pure read, no export-status transition, no `409` for an empty selection —
// see PreviewDocument's doc comment for why an in-progress edit is not a
// refusal the way an export attempt is.
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

// rerun is `POST /v1/generations/{runId}/rerun` (rest-api.md): `202` with
// the same run id. The body is optional — an empty or absent body reruns the
// whole run, matching RerunGenerationRequestDto's `omitempty` Sections field.
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
