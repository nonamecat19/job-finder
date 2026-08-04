package http

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/httpx"
)

// DocumentGenerator is the interface DocumentsHandler needs from the generation service.
type DocumentGenerator interface {
	GenerateAdHoc(ctx context.Context, in generation.AdHocInput) (resume, coverLetter dto.GeneratedDocumentDto, err error)
	ListAdHocDocuments(ctx context.Context) ([]dto.GeneratedDocumentDto, error)
	GetDocumentDto(ctx context.Context, id string) (dto.GeneratedDocumentDto, error)
	UpdateDocument(ctx context.Context, id, text string) (dto.GeneratedDocumentDto, error)
	GetDocumentDownload(ctx context.Context, id string) (path *string, filename string, err error)
}

// DocumentsHandler wires /api/documents, mirroring documents.controller.ts.
type DocumentsHandler struct {
	Generation DocumentGenerator
}

func (h *DocumentsHandler) Mount(r chi.Router) {
	r.Post("/documents/tailor", h.tailor)
	r.Get("/documents/ad-hoc", h.listAdHoc)
	r.Get("/documents/{id}", h.get)
	r.Put("/documents/{id}", h.update)
	r.Get("/documents/{id}/pdf", h.pdf)
}

type tailorBody struct {
	Vacancy         string   `json:"vacancy"`
	Company         string   `json:"company"`
	Title           string   `json:"title"`
	GroundingLevel  *string  `json:"groundingLevel"`
	RequiredSkills  []string `json:"requiredSkills,omitempty"`
	NiceToHave      []string `json:"niceToHave,omitempty"`
	ExperienceLevel *string  `json:"experienceLevel,omitempty"` // junior|mid|senior|lead|staff|principal
}

// tailor is the ad-hoc RenderCV tailoring endpoint from a pasted vacancy (no
// DB job) — the entry point the `/tailor-resume` command calls.
func (h *DocumentsHandler) tailor(w http.ResponseWriter, r *http.Request) {
	var body tailorBody
	if err := httpx.DecodeJSON(r, &body); err != nil || body.Vacancy == "" {
		httpx.WriteError(w, http.StatusBadRequest, "vacancy text is required")
		return
	}
	var level *generation.GroundingLevel
	if body.GroundingLevel != nil {
		l := generation.ParseGroundingLevel(*body.GroundingLevel)
		level = &l
	}
	var hints *generation.VacancyHints
	if len(body.RequiredSkills) > 0 || len(body.NiceToHave) > 0 || body.ExperienceLevel != nil {
		hints = &generation.VacancyHints{
			RequiredSkills: body.RequiredSkills,
			NiceToHave:     body.NiceToHave,
		}
		if body.ExperienceLevel != nil {
			hints.ExperienceLevel = *body.ExperienceLevel
		}
	}
	resume, coverLetter, err := h.Generation.GenerateAdHoc(r.Context(), generation.AdHocInput{
		Vacancy: body.Vacancy, Company: body.Company, Title: body.Title, GroundingLevel: level, Hints: hints,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"resume":      resume,
		"coverLetter": coverLetter,
	})
}

func (h *DocumentsHandler) listAdHoc(w http.ResponseWriter, r *http.Request) {
	out, err := h.Generation.ListAdHocDocuments(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *DocumentsHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Generation.GetDocumentDto(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type updateDocumentBody struct {
	Text string `json:"text"`
}

func (h *DocumentsHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body updateDocumentBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Generation.UpdateDocument(r.Context(), id, body.Text)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// downloadDisposition builds the Content-Disposition value for a PDF.
// filename= carries only ASCII, since anything else is undefined there; a name
// with non-ASCII characters (a Cyrillic or accented surname) additionally gets
// an RFC 5987 filename*, which every current browser prefers. When stripping
// leaves nothing usable, the ASCII parameter degrades to a generic name rather
// than an empty one, and filename* still carries the real spelling.
func downloadDisposition(filename string) string {
	var ascii strings.Builder
	for _, r := range filename {
		if r < 0x80 && r != '"' && r != '\\' && r >= 0x20 {
			ascii.WriteRune(r)
		}
	}
	fallback := strings.Trim(ascii.String(), "_")
	if fallback == "" || fallback == ".pdf" {
		fallback = "document.pdf"
	}
	disposition := `attachment; filename="` + fallback + `"`
	if fallback != filename {
		disposition += `; filename*=UTF-8''` + url.PathEscape(filename)
	}
	return disposition
}

func (h *DocumentsHandler) pdf(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pdfPath, filename, err := h.Generation.GetDocumentDownload(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if pdfPath == nil {
		httpx.WriteError(w, http.StatusNotFound, "PDF not rendered yet")
		return
	}
	if _, err := os.Stat(*pdfPath); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "PDF not rendered yet")
		return
	}
	if filename == "" {
		filename = filepath.Base(*pdfPath)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", downloadDisposition(filename))
	http.ServeFile(w, r, *pdfPath)
}
