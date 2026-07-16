package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/profile"
)

// ProfileProvider is the interface ProfilesHandler needs from the profile service.
type ProfileProvider interface {
	List(ctx context.Context) ([]dto.ProfileDto, error)
	GetDto(ctx context.Context, id string) (dto.ProfileDto, error)
	Create(ctx context.Context, name string, rendercvYaml string, extraNotes *string) (dto.ProfileDto, error)
	Update(ctx context.Context, id string, in profile.UpdateInput) (dto.ProfileDto, error)
	Remove(ctx context.Context, id string) error
	SaveConfig(ctx context.Context, yamlText string) (dto.ProfileDto, error)
}

// ProfilesHandler wires /api/profiles, mirroring profiles.controller.ts.
type ProfilesHandler struct {
	Profiles ProfileProvider
}

func (h *ProfilesHandler) Mount(r chi.Router) {
	r.Get("/profiles", h.list)
	r.Get("/profiles/{id}", h.get)
	r.Post("/profiles", h.create)
	r.Put("/profiles/{id}", h.update)
	r.Delete("/profiles/{id}", h.remove)
	r.Post("/profiles/config", h.uploadConfig)
	r.Get("/profiles/config/status", h.configStatus)
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
	Name         string  `json:"name"`
	RendercvYaml string  `json:"rendercvYaml"`
	ExtraNotes   *string `json:"extraNotes"`
}

func (h *ProfilesHandler) create(w http.ResponseWriter, r *http.Request) {
	var body createProfileBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Profiles.Create(r.Context(), body.Name, body.RendercvYaml, body.ExtraNotes)
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
	if v, ok := raw["rendercvYaml"]; ok {
		if s, ok := v.(string); ok {
			in.RendercvYaml = &s
		}
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

func (h *ProfilesHandler) uploadConfig(w http.ResponseWriter, r *http.Request) {
	var yamlText string

	// Check if it's multipart/form-data
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "multipart field 'file' is required")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read uploaded file")
			return
		}
		yamlText = string(data)
	} else {
		// Read raw body
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		yamlText = string(data)
	}

	if strings.TrimSpace(yamlText) == "" {
		writeError(w, http.StatusBadRequest, "yaml content is empty")
		return
	}

	// Process the config
	out, err := h.Profiles.SaveConfig(r.Context(), yamlText)
	if err != nil {
		// If validation/render fails, reject with 422
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, out)
}

func (h *ProfilesHandler) configStatus(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.Profiles.List(r.Context())
	if err != nil || len(profiles) == 0 {
		writeJSON(w, http.StatusOK, map[string]bool{"hasConfig": false})
		return
	}
	// Since we got at least one profile, check if hasConfig is true
	has := false
	for _, p := range profiles {
		if p.HasConfig {
			has = true
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"hasConfig": has})
}
