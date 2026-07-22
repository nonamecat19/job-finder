package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/outreach"
)

// OutreachProvider is the interface OutreachHandler needs from the
// outreach service (012). Read-and-produce only per Constitution
// Principle I: neither route sends, schedules, or queues anything.
type OutreachProvider interface {
	GenerateDraft(ctx context.Context, jobID, contactID, tone string) (dto.OutreachDraftDto, error)
	Tones() []dto.OutreachToneOptionDto
}

// OutreachHandler wires /api/jobs/{id}/outreach/generate and
// /api/jobs/{id}/outreach/tones (spec 012). generate is a POST (not a GET)
// because it may run a live LLM call, mirroring CoachHandler.assess — that
// cost never runs inline on a GET.
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
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	out, err := h.Outreach.GenerateDraft(r.Context(), jobID, body.ContactID, body.Tone)
	if err != nil {
		switch {
		case errors.Is(err, outreach.ErrContactNotFound):
			writeError(w, http.StatusNotFound, "contact not found for this job")
		case errors.Is(err, outreach.ErrContactRequired):
			writeError(w, http.StatusConflict, "multiple contacts resolved for this job — choose one via contactId")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *OutreachHandler) tones(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Outreach.Tones())
}
