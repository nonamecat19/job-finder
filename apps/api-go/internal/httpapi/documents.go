package httpapi

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api-go/internal/generation"
)

// DocumentsHandler wires /api/documents, mirroring documents.controller.ts.
type DocumentsHandler struct {
	Generation *generation.Service
}

func (h *DocumentsHandler) Mount(r chi.Router) {
	r.Post("/documents/tailor", h.tailor)
	r.Get("/documents/{id}", h.get)
	r.Put("/documents/{id}", h.update)
	r.Get("/documents/{id}/pdf", h.pdf)
}

type tailorBody struct {
	Vacancy        string  `json:"vacancy"`
	Company        string  `json:"company"`
	Title          string  `json:"title"`
	GroundingLevel *string `json:"groundingLevel"`
}

// tailor is the ad-hoc RenderCV tailoring endpoint from a pasted vacancy (no
// DB job) — the entry point the opencode `/tailor-resume` command calls.
func (h *DocumentsHandler) tailor(w http.ResponseWriter, r *http.Request) {
	var body tailorBody
	if err := decodeJSON(r, &body); err != nil || body.Vacancy == "" {
		writeError(w, http.StatusBadRequest, "vacancy text is required")
		return
	}
	var level *generation.GroundingLevel
	if body.GroundingLevel != nil {
		l := generation.ParseGroundingLevel(*body.GroundingLevel)
		level = &l
	}
	result, err := h.Generation.GenerateRendercvFromText(r.Context(), generation.RendercvFromTextInput{
		Vacancy: body.Vacancy, Company: body.Company, Title: body.Title, GroundingLevel: level,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"yamlPath":       result.YamlPath,
		"pdfPath":        result.PdfPath,
		"groundingLevel": result.GroundingLevel,
	})
}

func (h *DocumentsHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Generation.GetDocumentDto(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type updateDocumentBody struct {
	Text string `json:"text"`
}

func (h *DocumentsHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body updateDocumentBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Generation.UpdateDocument(r.Context(), id, body.Text)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *DocumentsHandler) pdf(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	doc, err := h.Generation.GetDocument(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if doc.PdfPath == nil {
		writeError(w, http.StatusNotFound, "PDF not rendered yet")
		return
	}
	if _, err := os.Stat(*doc.PdfPath); err != nil {
		writeError(w, http.StatusNotFound, "PDF not rendered yet")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(*doc.PdfPath)+`"`)
	http.ServeFile(w, r, *doc.PdfPath)
}
