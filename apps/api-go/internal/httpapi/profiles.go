package httpapi

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/profile"
)

// ProfilesHandler wires /api/profiles, mirroring profile.controller.ts.
type ProfilesHandler struct {
	Profiles *profile.Service
}

func (h *ProfilesHandler) Mount(r chi.Router) {
	r.Get("/profiles", h.list)
	r.Get("/profiles/{id}", h.get)
	r.Post("/profiles", h.create)
	r.Put("/profiles/{id}", h.update)
	r.Delete("/profiles/{id}", h.remove)
	r.Post("/profiles/import", h.importPdf)
}

func (h *ProfilesHandler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.Profiles.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProfilesHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Profiles.GetDto(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type createProfileBody struct {
	Name       string         `json:"name"`
	Document   dto.JsonResume `json:"document"`
	ExtraNotes *string        `json:"extraNotes"`
}

func (h *ProfilesHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createProfileBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Profiles.Create(r.Context(), body.Name, body.Document, body.ExtraNotes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProfilesHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var raw map[string]any
	if err := decodeJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	in := profile.UpdateInput{}
	if v, ok := raw["name"]; ok {
		if s, ok := v.(string); ok {
			in.Name = &s
		}
	}
	if v, ok := raw["document"]; ok {
		doc := reencode[dto.JsonResume](v)
		in.Document = &doc
	}
	if v, ok := raw["extraNotes"]; ok {
		var notes *string
		if s, ok := v.(string); ok {
			notes = &s
		}
		in.ExtraNotes = &notes
	}

	out, err := h.Profiles.Update(r.Context(), id, in)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ProfilesHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Profiles.Remove(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// importPdf accepts a multipart/form-data upload with field "file" (PDF),
// mirroring @UseInterceptors(FileInterceptor('file')).
func (h *ProfilesHandler) importPdf(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeError(w, http.StatusBadRequest, `multipart field "file" (PDF) is required`)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, `multipart field "file" (PDF) is required`)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read uploaded file")
		return
	}
	result, err := h.Profiles.ImportPdf(r.Context(), data)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": result.Draft, "textLength": result.TextLength})
}
