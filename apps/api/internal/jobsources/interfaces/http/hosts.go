package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

type HostRetrievalProvider interface {
	HostStatus(ctx context.Context, host string) (dto.HostRetrievalStatusDto, error)
	ClearRungPreference(ctx context.Context, host string) error
	ClearCookies(ctx context.Context, host string) error
	OverrideCoolingOff(ctx context.Context, host string) (time.Duration, error)
}

type HostsHandler struct {
	Retrieval HostRetrievalProvider
}

var errHostNotFound = errors.New("host not found")

func (h *HostsHandler) Mount(r chi.Router) {
	r.Get("/hosts/{host}/retrieval-status", h.retrievalStatus)
	r.Post("/hosts/{host}/clear-rung-preference", h.clearRungPreference)
	r.Post("/hosts/{host}/clear-cookies", h.clearCookies)
	r.Post("/hosts/{host}/override-cooling-off", h.overrideCoolingOff)
}

func (h *HostsHandler) retrievalStatus(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	status, err := h.Retrieval.HostStatus(r.Context(), host)
	if err != nil {
		if errors.Is(err, errHostNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "host not found: "+host)
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

func (h *HostsHandler) clearRungPreference(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if err := h.Retrieval.ClearRungPreference(r.Context(), host); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HostsHandler) clearCookies(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	if err := h.Retrieval.ClearCookies(r.Context(), host); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HostsHandler) overrideCoolingOff(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	remaining, err := h.Retrieval.OverrideCoolingOff(r.Context(), host)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]float64{
		"remainingSeconds": remaining.Seconds(),
	})
}
