package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
)

// SourcesProvider is the interface SourcesHandler needs from the job-sources service.
type SourcesProvider interface {
	List(ctx context.Context) ([]dto.JobSourceDto, error)
	Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
	Test(ctx context.Context, key string) (bool, string)
}

// EnrichmentRunner enqueues backfill enrichment tasks for a source.
type EnrichmentRunner interface {
	EnqueueBackfill(ctx context.Context, sourceKey string, limit int32) (int, error)
}

// SourceRunner enqueues a direct "run this source" ingest task.
type SourceRunner interface {
	RunSource(ctx context.Context, sourceKey string) error
}

// SourcesHandler wires /api/sources, mirroring job-sources.controller.ts.
type SourcesHandler struct {
	Sources    SourcesProvider
	Enrichment EnrichmentRunner
	Ingestion  SourceRunner
}

func (h *SourcesHandler) Mount(r chi.Router) {
	r.Get("/sources", h.list)
	r.Put("/sources/{key}", h.update)
	r.Post("/sources/{key}/test", h.test)
	r.Post("/sources/{key}/run", h.run)
	r.Post("/sources/{key}/enrich", h.enrich)
}

func (h *SourcesHandler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.Sources.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type updateSourceBody struct {
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (h *SourcesHandler) update(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var body updateSourceBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Sources.Update(r.Context(), key, body.Enabled, body.Config)
	if err != nil {
		writeError(w, http.StatusNotFound, "source not found: "+key)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SourcesHandler) test(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	ok, errMsg := h.Sources.Test(r.Context(), key)
	resp := map[string]any{"ok": ok}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	writeJSON(w, http.StatusOK, resp)
}

// run enqueues a single ingest task for the given source with no saved search
// or subscription — a direct "run this source" trigger.
func (h *SourcesHandler) run(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := h.Ingestion.RunSource(r.Context(), key); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enqueued": []string{key}})
}

// enrich queues an "enrich" task for every shallow (detailScrapedAt IS NULL)
// row on this source — a backfill sweep for links ingested with no full
// data. Only djinni and dou implement detail fetching today.
func (h *SourcesHandler) enrich(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key != "djinni" && key != "dou" {
		writeError(w, http.StatusBadRequest, "detail enrichment is only implemented for 'djinni' and 'dou'")
		return
	}
	limit := int32(200)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = int32(n)
		}
	}
	n, err := h.Enrichment.EnqueueBackfill(r.Context(), key, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"enqueued": n})
}
