package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/httpx"
)

type SummaryModelProvider interface {
	Get() domain.SummaryOption
	Options() []domain.SummaryOption
	Update(ctx context.Context, optionID string) (domain.SummaryOption, error)
	Reset(ctx context.Context) (domain.SummaryOption, error)
}

type SummaryModelHandler struct {
	Settings SummaryModelProvider
}

func (h *SummaryModelHandler) Mount(r chi.Router) {
	r.Get("/settings/summary-model", h.get)
	r.Put("/settings/summary-model", h.update)
	r.Delete("/settings/summary-model", h.reset)
}

func settingToDto(options []domain.SummaryOption, current domain.SummaryOption) dto.SummaryModelSettingDto {
	out := dto.SummaryModelSettingDto{
		Options:  make([]dto.SummaryModelOptionDto, 0, len(options)),
		OptionID: current.ID,
	}
	for _, o := range options {
		out.Options = append(out.Options, dto.SummaryModelOptionDto{
			ID:          o.ID,
			Label:       o.Label,
			Description: o.Description,
			Cost:        o.Cost,
			Current:     o.ID == current.ID,
		})
	}
	return out
}

func (h *SummaryModelHandler) get(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, settingToDto(h.Settings.Options(), h.Settings.Get()))
}

func (h *SummaryModelHandler) update(w http.ResponseWriter, r *http.Request) {
	var body dto.UpdateSummaryModelRequestDto
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	stored, err := h.Settings.Update(r.Context(), body.OptionID)
	if err != nil {

		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settingToDto(h.Settings.Options(), stored))
}

func (h *SummaryModelHandler) reset(w http.ResponseWriter, r *http.Request) {
	stored, err := h.Settings.Reset(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, settingToDto(h.Settings.Options(), stored))
}
