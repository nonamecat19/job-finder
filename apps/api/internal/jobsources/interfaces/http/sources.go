package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

type SourcesProvider interface {
	List(ctx context.Context) ([]dto.JobSourceDto, error)
	Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
	Test(ctx context.Context, key string) (bool, string)
}

type EnrichmentRunner interface {
	EnqueueBackfill(ctx context.Context, sourceKey string, limit int32) (int, error)
}

type SourceRunner interface {
	RunSource(ctx context.Context, sourceKey string) error
}

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
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type updateSourceBody struct {
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (h *SourcesHandler) update(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var body updateSourceBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Sources.Update(r.Context(), key, body.Enabled, body.Config)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "source not found: "+key)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *SourcesHandler) test(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	ok, errMsg := h.Sources.Test(r.Context(), key)
	resp := map[string]any{"ok": ok}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *SourcesHandler) run(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := h.Ingestion.RunSource(r.Context(), key); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enqueued": []string{key}})
}

func (h *SourcesHandler) enrich(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key != "djinni" && key != "dou" {
		httpx.WriteError(w, http.StatusBadRequest, "detail enrichment is only implemented for 'djinni' and 'dou'")
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
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"enqueued": n})
}
