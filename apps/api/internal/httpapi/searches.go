package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/ingestion"
)

// SearchProvider is the interface SearchesHandler needs from the ingestion service.
type SearchProvider interface {
	ListSearches(ctx context.Context) ([]dto.SavedSearchDto, error)
	CreateSearch(ctx context.Context, name string, query dto.SearchQuery, cron string, enabled bool) (*dto.SavedSearchDto, error)
	UpdateSearch(ctx context.Context, id string, in ingestion.UpdateSearchInput) (*dto.SavedSearchDto, error)
	DeleteSearch(ctx context.Context, id string) error
	RunSearch(ctx context.Context, searchID string) ([]string, error)
	RecentRuns(ctx context.Context, limit int32) ([]dto.SourceRunDto, error)
}

// SearchesHandler wires /api/searches, mirroring searches.controller.ts.
type SearchesHandler struct {
	Ingestion SearchProvider
}

func (h *SearchesHandler) Mount(r chi.Router) {
	r.Get("/searches", h.list)
	r.Post("/searches", h.create)
	r.Put("/searches/{id}", h.update)
	r.Delete("/searches/{id}", h.remove)
	r.Post("/searches/{id}/run", h.run)
	r.Get("/searches/runs/recent", h.recentRuns)
}

func (h *SearchesHandler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.Ingestion.ListSearches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type createSearchBody struct {
	Name    string          `json:"name"`
	Query   dto.SearchQuery `json:"query"`
	Cron    string          `json:"cron"`
	Enabled *bool           `json:"enabled"`
}

func (h *SearchesHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createSearchBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	out, err := h.Ingestion.CreateSearch(r.Context(), body.Name, body.Query, body.Cron, enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type updateSearchBody struct {
	Name    *string          `json:"name"`
	Query   *dto.SearchQuery `json:"query"`
	Cron    *string          `json:"cron"`
	Enabled *bool            `json:"enabled"`
}

func (h *SearchesHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body updateSearchBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Ingestion.UpdateSearch(r.Context(), id, ingestion.UpdateSearchInput{
		Name: body.Name, Query: body.Query, Cron: body.Cron, Enabled: body.Enabled,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "search not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SearchesHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Ingestion.DeleteSearch(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *SearchesHandler) run(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	keys, err := h.Ingestion.RunSearch(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "search not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enqueued": keys})
}

func (h *SearchesHandler) recentRuns(w http.ResponseWriter, r *http.Request) {
	out, err := h.Ingestion.RecentRuns(r.Context(), 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
