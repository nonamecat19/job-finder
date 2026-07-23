package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/companyintel/application"
	"github.com/job-finder/api/internal/dto"
)

// CompanyIntelProvider is the interface CompaniesHandler needs from the
// companyintel service.
type CompanyIntelProvider interface {
	GetIntel(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error)
	Refresh(ctx context.Context, jobID string) (*dto.CompanyIntelDto, error)
}

// CompaniesHandler wires /api/companies/{jobId}/intel and
// /api/companies/{jobId}/intel/refresh, mirroring the dashboard's
// api.companies client (spec 004).
type CompaniesHandler struct {
	CompanyIntel CompanyIntelProvider
}

func (h *CompaniesHandler) Mount(r chi.Router) {
	r.Get("/companies/{jobId}/intel", h.intel)
	r.Post("/companies/{jobId}/intel/refresh", h.refresh)
}

func (h *CompaniesHandler) intel(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	out, err := h.CompanyIntel.GetIntel(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// out is nil when the company has never been probed (or the job has no
	// parseable company name) — serialized as JSON null, which the
	// dashboard's `!intel` check renders as the "no data yet" state.
	writeJSON(w, http.StatusOK, out)
}

func (h *CompaniesHandler) refresh(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobId")
	out, err := h.CompanyIntel.Refresh(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, application.ErrNoCompany) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
