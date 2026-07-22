package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/referral"
)

// ReferralProvider is the interface ReferralHandler needs from the referral
// service.
type ReferralProvider interface {
	ImportCSV(ctx context.Context, r io.Reader) (referral.ImportSummary, error)
	SyncGithub(ctx context.Context, contactID string) (*referral.GithubSyncResult, error)
	FindReferralPaths(ctx context.Context, jobID string, maxDepth, topN int) ([]referral.ReferralPath, error)
	ListContacts(ctx context.Context) ([]referral.Contact, error)
}

// ReferralHandler wires POST /api/contacts/import, POST
// /api/contacts/{id}/github-sync, and GET /api/jobs/{id}/referral-paths —
// the referral-path-finder feature (plan 011).
type ReferralHandler struct {
	Referral ReferralProvider
}

func (h *ReferralHandler) Mount(r chi.Router) {
	r.Get("/contacts", h.list)
	r.Post("/contacts/import", h.importCSV)
	r.Post("/contacts/{id}/github-sync", h.githubSync)
	r.Get("/jobs/{id}/referral-paths", h.referralPaths)
}

func (h *ReferralHandler) list(w http.ResponseWriter, r *http.Request) {
	contacts, err := h.Referral.ListContacts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]dto.ReferralContactDto, 0, len(contacts))
	for _, c := range contacts {
		out = append(out, contactToDto(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// importCSV accepts a raw text/csv body, or a multipart/form-data upload
// with the file under the "file" field — matching the profile config
// upload convention (ProfilesHandler.uploadConfig).
func (h *ReferralHandler) importCSV(w http.ResponseWriter, r *http.Request) {
	var body io.Reader = r.Body
	defer r.Body.Close()

	if ct := r.Header.Get("Content-Type"); len(ct) >= 19 && ct[:19] == "multipart/form-data" {
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing file field")
			return
		}
		defer file.Close()
		body = file
	}

	out, err := h.Referral.ImportCSV(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto.ContactImportResultDto{
		Imported: out.Imported,
		Skipped:  out.Skipped,
		Total:    out.Total,
	})
}

func (h *ReferralHandler) githubSync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Referral.SyncGithub(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, referral.ErrContactNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, referral.ErrNoGithubUsername):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, dto.GithubSyncResultDto{
		Contact:          contactToDto(out.Contact),
		FollowersScanned: out.FollowersScanned,
		FollowingScanned: out.FollowingScanned,
		ConnectionsMade:  out.ConnectionsMade,
	})
}

func (h *ReferralHandler) referralPaths(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	maxDepth := 0
	if v := r.URL.Query().Get("maxDepth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxDepth = n
		}
	}
	topN := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			topN = n
		}
	}

	paths, err := h.Referral.FindReferralPaths(r.Context(), id, maxDepth, topN)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]dto.ReferralPathDto, 0, len(paths))
	for _, p := range paths {
		out = append(out, pathToDto(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func contactToDto(c referral.Contact) dto.ReferralContactDto {
	return dto.ReferralContactDto{
		ID:             c.ID,
		Name:           c.Name,
		Email:          c.Email,
		Company:        c.Company,
		Role:           c.Role,
		LinkedInUrl:    c.LinkedInURL,
		GitHubUsername: c.GitHubUsername,
	}
}

func pathToDto(p referral.ReferralPath) dto.ReferralPathDto {
	contacts := make([]dto.ReferralContactDto, 0, len(p.Path))
	for _, c := range p.Path {
		contacts = append(contacts, contactToDto(c))
	}
	return dto.ReferralPathDto{
		Path:   contacts,
		Score:  p.Score,
		Length: p.Length,
	}
}
