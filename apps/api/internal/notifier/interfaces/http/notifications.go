package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

type NotificationProvider interface {
	ListNotifications(ctx context.Context) ([]dto.FreshMatchNotificationDto, error)
	MarkSeen(ctx context.Context, id string) error
	UnseenCount(ctx context.Context) (int64, error)
}

type NotificationHandler struct {
	Provider NotificationProvider
}

func (h *NotificationHandler) Mount(r chi.Router) {
	r.Get("/notifications", h.list)
	r.Post("/notifications/{id}/seen", h.markSeen)
	r.Get("/notifications/unseen-count", h.unseenCount)
}

func (h *NotificationHandler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.Provider.ListNotifications(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *NotificationHandler) markSeen(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Provider.MarkSeen(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationHandler) unseenCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.Provider.UnseenCount(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"count": count})
}
