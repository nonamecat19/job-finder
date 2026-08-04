package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/httpx"
)

// ShapeProvider is the interface ResumeShapeHandler needs from the resumeshape
// service. *resumeshape.Service satisfies it structurally.
type ShapeProvider interface {
	Get() domain.ShapeConfig
	Update(ctx context.Context, cfg domain.ShapeConfig) (domain.ShapeConfig, error)
	Reset(ctx context.Context) (domain.ShapeConfig, error)
}

// ResumeShapeHandler exposes GET/PUT/DELETE /v1/settings/resume-shape: the
// shape of every generated resume (summary length, bullets per job, optional
// sections, project limits, page target). PUT replaces the whole config —
// validation runs before any write, so a rejected payload stores nothing.
// DELETE means "drop my overrides" and returns the defaults.
type ResumeShapeHandler struct {
	Settings ShapeProvider
}

func (h *ResumeShapeHandler) Mount(r chi.Router) {
	r.Get("/settings/resume-shape", h.get)
	r.Put("/settings/resume-shape", h.update)
	r.Delete("/settings/resume-shape", h.reset)
}

func configToDto(c domain.ShapeConfig) dto.ResumeShapeConfigDto {
	return dto.ResumeShapeConfigDto{
		SummaryLines:          c.SummaryLines,
		SkillsEnabled:         c.SkillsEnabled,
		SkillsMaxGroups:       c.SkillsMaxGroups,
		ExperienceBulletsMin:  c.ExperienceBulletsMin,
		ExperienceBulletsMax:  c.ExperienceBulletsMax,
		TargetPages:           c.TargetPages,
		ProjectsEnabled:       c.ProjectsEnabled,
		ProjectsMin:           c.ProjectsMin,
		ProjectsMax:           c.ProjectsMax,
		ProjectBulletsMax:     c.ProjectBulletsMax,
		CertificationsEnabled: c.CertificationsEnabled,
		CertificationsMin:     c.CertificationsMin,
		CertificationsMax:     c.CertificationsMax,
		FontSize:              c.FontSize,
	}
}

func dtoToConfig(d dto.ResumeShapeConfigDto) domain.ShapeConfig {
	return domain.ShapeConfig{
		SummaryLines:          d.SummaryLines,
		SkillsEnabled:         d.SkillsEnabled,
		SkillsMaxGroups:       d.SkillsMaxGroups,
		ExperienceBulletsMin:  d.ExperienceBulletsMin,
		ExperienceBulletsMax:  d.ExperienceBulletsMax,
		TargetPages:           d.TargetPages,
		ProjectsEnabled:       d.ProjectsEnabled,
		ProjectsMin:           d.ProjectsMin,
		ProjectsMax:           d.ProjectsMax,
		ProjectBulletsMax:     d.ProjectBulletsMax,
		CertificationsEnabled: d.CertificationsEnabled,
		CertificationsMin:     d.CertificationsMin,
		CertificationsMax:     d.CertificationsMax,
		FontSize:              d.FontSize,
	}
}

func (h *ResumeShapeHandler) get(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, configToDto(h.Settings.Get()))
}

func (h *ResumeShapeHandler) update(w http.ResponseWriter, r *http.Request) {
	var body dto.ResumeShapeConfigDto
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg := dtoToConfig(body)
	// Validate here as well as in the service so a bad payload is a 400 rather
	// than a 500 — the service's own check is what makes the write atomic.
	if err := cfg.Validate(); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	stored, err := h.Settings.Update(r.Context(), cfg)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, configToDto(stored))
}

func (h *ResumeShapeHandler) reset(w http.ResponseWriter, r *http.Request) {
	stored, err := h.Settings.Reset(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, configToDto(stored))
}
