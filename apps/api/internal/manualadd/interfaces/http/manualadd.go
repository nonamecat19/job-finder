package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/manualadd/application"
	"github.com/job-finder/api/internal/manualadd/domain"
)

type AddService interface {
	Add(ctx context.Context, rawURL string) (domain.Result, error)
	SaveFillIn(ctx context.Context, in application.FillIn) (domain.Result, error)
}

type ManualAddHandler struct {
	Manual AddService
}

func (h *ManualAddHandler) Mount(r chi.Router) {
	r.Post("/jobs/manual", h.add)
	r.Post("/jobs/manual/fill-in", h.fillIn)
}

type manualAddBody struct {
	URL string `json:"url"`
}

func (h *ManualAddHandler) add(w http.ResponseWriter, r *http.Request) {
	var body manualAddBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "url is required")
		return
	}

	result, err := h.Manual.Add(r.Context(), body.URL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, statusFor(result), ResultDto(result))
}

type manualFillInBody struct {
	URL         string  `json:"url"`
	SourceKey   *string `json:"sourceKey"`
	Title       string  `json:"title"`
	Company     string  `json:"company"`
	Description string  `json:"description"`
	Location    *string `json:"location"`
	Remote      bool    `json:"remote"`
	SalaryRaw   *string `json:"salaryRaw"`
	PostedAt    *string `json:"postedAt"`
}

func (h *ManualAddHandler) fillIn(w http.ResponseWriter, r *http.Request) {
	var body manualFillInBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}

	result, err := h.Manual.SaveFillIn(r.Context(), application.FillIn{
		URL:         body.URL,
		SourceKey:   body.SourceKey,
		Title:       body.Title,
		Company:     body.Company,
		Description: body.Description,
		Location:    body.Location,
		Remote:      body.Remote,
		SalaryRaw:   body.SalaryRaw,
		PostedAt:    body.PostedAt,
	})
	if err != nil {
		var missing *application.MissingFieldsError
		if errors.As(err, &missing) {
			httpx.WriteError(w, http.StatusBadRequest, missing.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, statusFor(result), ResultDto(result))
}

func statusFor(result domain.Result) int {
	switch result.Outcome {
	case domain.OutcomeCreated:
		return http.StatusCreated
	case domain.OutcomeDuplicate, domain.OutcomeNeedsFillIn:
		return http.StatusOK
	default:
		if result.Kind == domain.KindInvalidURL {
			return http.StatusBadRequest
		}
		return http.StatusUnprocessableEntity
	}
}

func ResultDto(result domain.Result) dto.ManualAddResultDto {
	out := dto.ManualAddResultDto{Outcome: string(result.Outcome), Job: result.Job}
	if result.Reason != "" {
		reason := result.Reason
		out.Reason = &reason
	}
	if result.Kind != "" {
		kind := string(result.Kind)
		out.Kind = &kind
	}
	if result.Draft != nil {
		out.Draft = draftDto(result.Draft)
	}
	return out
}

func draftDto(d *domain.Draft) *dto.ManualVacancyDraftDto {
	return &dto.ManualVacancyDraftDto{
		URL:         d.URL,
		SourceKey:   d.SourceKey,
		Title:       d.Title,
		Company:     d.Company,
		Location:    d.Location,
		Remote:      d.Remote,
		SalaryRaw:   d.SalaryRaw,
		Description: d.Description,
		PostedAt:    d.PostedAt,
	}
}
