package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
)

// ActivityProvider is the interface ActivityHandler needs from the
// activity-tracking service. *activityapp.Service satisfies it structurally.
type ActivityProvider interface {
	List(ctx context.Context, limit int32) (dto.ActivityListResponse, error)
	Retry(ctx context.Context, opFilter *string) (retried, skipped int, err error)
}

type ActivityHandler struct {
	svc ActivityProvider
}

func NewActivityHandler(svc ActivityProvider) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

func (h *ActivityHandler) Mount(r chi.Router) {
	r.Get("/activity", h.list)
	r.Post("/activity/retry", h.retry)
}

func (h *ActivityHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = int32(v)
		}
	}

	resp, err := h.svc.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list activity runs: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// retry re-enqueues every failed ActivityRun, or only those matching
// ?op=<op> when given.
func (h *ActivityHandler) retry(w http.ResponseWriter, r *http.Request) {
	var opFilter *string
	if op := r.URL.Query().Get("op"); op != "" {
		opFilter = &op
	}

	retried, skipped, err := h.svc.Retry(r.Context(), opFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "retry activity runs: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"retried": retried, "skipped": skipped})
}
