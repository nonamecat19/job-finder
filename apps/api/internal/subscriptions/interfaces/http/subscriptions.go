package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/subscriptions"
)

type SubscriptionProvider interface {
	List(ctx context.Context) ([]dto.SubscriptionDto, error)
	ListBySource(ctx context.Context, sourceKey string) ([]dto.SubscriptionDto, error)
	Create(ctx context.Context, sourceKey, url string, name *string, enabled bool, cron string) (*dto.SubscriptionDto, error)
	Update(ctx context.Context, id string, in subscriptions.UpdateInput) (*dto.SubscriptionDto, error)
	Delete(ctx context.Context, id string) error
}

type SubscriptionRunner interface {
	RunSubscription(ctx context.Context, subID string) error
	RunAllSubscriptions(ctx context.Context) (int, error)
}

type SubscriptionsHandler struct {
	Subs      SubscriptionProvider
	Ingestion SubscriptionRunner
}

func (h *SubscriptionsHandler) Mount(r chi.Router) {
	r.Get("/subscriptions", h.list)
	r.Post("/subscriptions", h.create)
	r.Put("/subscriptions/{id}", h.update)
	r.Delete("/subscriptions/{id}", h.remove)
	r.Post("/subscriptions/{id}/run", h.run)
	r.Post("/subscriptions/run-all", h.runAll)
}

func (h *SubscriptionsHandler) list(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source != "" {
		out, err := h.Subs.ListBySource(r.Context(), source)
		if err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	out, err := h.Subs.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type createSubscriptionBody struct {
	SourceKey string  `json:"sourceKey"`
	URL       string  `json:"url"`
	Name      *string `json:"name"`
	Enabled   *bool   `json:"enabled"`
	Cron      *string `json:"cron"`
}

func (h *SubscriptionsHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createSubscriptionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	var cron string
	if body.Cron != nil {
		cron = *body.Cron
	}
	out, err := h.Subs.Create(r.Context(), body.SourceKey, body.URL, body.Name, enabled, cron)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type updateSubscriptionBody struct {
	Name    *string `json:"name"`
	URL     *string `json:"url"`
	Enabled *bool   `json:"enabled"`
	Cron    *string `json:"cron"`
}

func (h *SubscriptionsHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body updateSubscriptionBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Subs.Update(r.Context(), id, subscriptions.UpdateInput{
		Name: body.Name, URL: body.URL, Enabled: body.Enabled, Cron: body.Cron,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "subscription not found: "+id)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *SubscriptionsHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Subs.Delete(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *SubscriptionsHandler) run(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Ingestion.RunSubscription(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"queued": true})
}

func (h *SubscriptionsHandler) runAll(w http.ResponseWriter, r *http.Request) {
	queued, err := h.Ingestion.RunAllSubscriptions(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int{"queued": queued})
}
