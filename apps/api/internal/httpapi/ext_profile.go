package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/extauth"
)

// ExtProfileProvider is the interface ExtProfileHandler needs from the
// profile service.
type ExtProfileProvider interface {
	ExtProfile(ctx context.Context) (dto.ExtProfileDto, error)
}

// TokenVerifier is the interface ExtProfileHandler needs to authenticate a
// bearer access token. *extauth.Signer satisfies it.
type TokenVerifier interface {
	Verify(token string) (*extauth.Claims, error)
}

// ExtProfileHandler wires GET /api/v1/ext/profile (spec section 3): the
// only extension-facing data endpoint, gated by a profile:read-scoped
// bearer token and always returning the token subject's own profile only
// (spec 3.4 — "must reject requests for any profile other than the
// authenticated user's").
type ExtProfileHandler struct {
	Profiles ExtProfileProvider
	Verifier TokenVerifier
}

func (h *ExtProfileHandler) Mount(r chi.Router) {
	r.Get("/ext/profile", h.get)
}

func (h *ExtProfileHandler) get(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	// extauth.Subject is the single profile owner in this single-tenant
	// deployment (see internal/extauth.Subject doc comment); this check is
	// the enforcement point for spec 3.4's cross-user rejection and stays
	// correct if a real per-user subject is introduced later.
	if claims.Subject != extauth.Subject {
		writeError(w, http.StatusForbidden, "token subject does not match profile owner")
		return
	}

	out, err := h.Profiles.ExtProfile(r.Context())
	if err != nil {
		writeError(w, http.StatusNotFound, "no profile configured")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// authenticate extracts and verifies the bearer access token. Never logs
// the token itself (only that a request was rejected) — auth tokens must
// never be logged, in request logs or anywhere else.
func (h *ExtProfileHandler) authenticate(w http.ResponseWriter, r *http.Request) (*extauth.Claims, bool) {
	authz := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(authz, prefix) {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return nil, false
	}
	token := strings.TrimPrefix(authz, prefix)
	claims, err := h.Verifier.Verify(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired access token")
		return nil, false
	}
	if claims.Scope != extauth.ScopeProfileRead {
		writeError(w, http.StatusForbidden, "token missing profile:read scope")
		return nil, false
	}
	return claims, true
}
