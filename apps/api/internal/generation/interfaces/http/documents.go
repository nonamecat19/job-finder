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

type DocumentGenerator interface {
	GetDocumentDto(ctx context.Context, id string) (dto.GeneratedDocumentDto, error)
	UpdateDocument(ctx context.Context, id, text string) (dto.GeneratedDocumentDto, error)
	GetDocumentDownload(ctx context.Context, id string) (path *string, filename string, err error)
}

type SummaryModelStore interface {
	Update(ctx context.Context, optionID string) (generation.SummaryOption, error)
}

type DocumentsHandler struct {
	Generation   DocumentGenerator
	SummaryModel SummaryModelStore
}

func (h *DocumentsHandler) Mount(r chi.Router) {
	r.Get("/documents/{id}", h.get)
	r.Put("/documents/{id}", h.update)
	r.Get("/documents/{id}/pdf", h.pdf)
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
