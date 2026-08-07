package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/outreach"
)

type OutreachProvider interface {
	GenerateDraft(ctx context.Context, jobID, contactID, tone string) (dto.OutreachDraftDto, error)
	Tones() []dto.OutreachToneOptionDto
}

type OutreachHandler struct {
	Outreach OutreachProvider
}

func (h *OutreachHandler) Mount(r chi.Router) {
	r.Post("/jobs/{id}/outreach/generate", h.generate)
	r.Get("/jobs/{id}/outreach/tones", h.tones)
}

type generateOutreachRequest struct {
	ContactID string `json:"contactId"`
	Tone      string `json:"tone"`
}

func (h *OutreachHandler) generate(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")

	var body generateOutreachRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &body); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	out, err := h.Outreach.GenerateDraft(r.Context(), jobID, body.ContactID, body.Tone)
	if err != nil {
		switch {
		case errors.Is(err, outreach.ErrContactNotFound):
			httpx.WriteError(w, http.StatusNotFound, "contact not found for job: "+jobID)
		case errors.Is(err, outreach.ErrContactRequired):
			httpx.WriteError(w, http.StatusConflict, "multiple contacts resolved for this job — choose one via contactId")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *OutreachHandler) tones(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, h.Outreach.Tones())
}
