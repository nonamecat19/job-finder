package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/extauth"
)

// ExtAuthProvider is the interface ExtAuthHandler needs from extauth.Service.
type ExtAuthProvider interface {
	IssueBootstrapCode(ctx context.Context) (code string, expiresAt time.Time, err error)
	ExchangeBootstrapCode(ctx context.Context, code string) (extauth.TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken string) (extauth.TokenPair, error)
}

// ExtAuthHandler wires the browser-extension auth endpoints (spec
// 014-autofill-extension section 2). Routes are mounted at /v1/ext/auth/*,
// giving /api/v1/ext/auth/... — the one versioned surface in this API,
// matching the spec exactly since it's a separate, independently-versioned
// client (the extension) rather than the dashboard.
type ExtAuthHandler struct {
	Auth ExtAuthProvider
}

func (h *ExtAuthHandler) Mount(r chi.Router) {
	r.Route("/v1/ext/auth", func(auth chi.Router) {
		// bootstrap is called by the dashboard (not the extension) to
		// generate the one-time code shown as a QR/string (spec 2.1 step 2).
		auth.Post("/bootstrap", h.bootstrap)
		auth.Post("/exchange", h.exchange)
		auth.Post("/refresh", h.refresh)
	})
}

type bootstrapResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

func (h *ExtAuthHandler) bootstrap(w http.ResponseWriter, r *http.Request) {
	code, expiresAt, err := h.Auth.IssueBootstrapCode(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to issue bootstrap code")
		return
	}
	writeJSON(w, http.StatusOK, bootstrapResponse{
		Code:      code,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	})
}

type exchangeBody struct {
	Code string `json:"code"`
}

type refreshBody struct {
	RefreshToken string `json:"refreshToken"`
}

type tokenPairResponse struct {
	AccessToken           string `json:"accessToken"`
	AccessTokenExpiresAt  string `json:"accessTokenExpiresAt"`
	RefreshToken          string `json:"refreshToken"`
	RefreshTokenExpiresAt string `json:"refreshTokenExpiresAt"`
	Scope                 string `json:"scope"`
}

func toTokenPairResponse(p extauth.TokenPair) tokenPairResponse {
	return tokenPairResponse{
		AccessToken:           p.AccessToken,
		AccessTokenExpiresAt:  p.AccessTokenExpiresAt.UTC().Format(time.RFC3339),
		RefreshToken:          p.RefreshToken,
		RefreshTokenExpiresAt: p.RefreshTokenExpiresAt.UTC().Format(time.RFC3339),
		Scope:                 p.Scope,
	}
}

func (h *ExtAuthHandler) exchange(w http.ResponseWriter, r *http.Request) {
	var body exchangeBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	pair, err := h.Auth.ExchangeBootstrapCode(r.Context(), body.Code)
	if err != nil {
		if errors.Is(err, extauth.ErrInvalidCode) {
			writeError(w, http.StatusUnauthorized, "invalid or expired bootstrap code")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to exchange bootstrap code")
		return
	}
	writeJSON(w, http.StatusOK, toTokenPairResponse(pair))
}

func (h *ExtAuthHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var body refreshBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	pair, err := h.Auth.RefreshTokens(r.Context(), body.RefreshToken)
	if err != nil {
		if errors.Is(err, extauth.ErrInvalidRefreshToken) {
			writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to refresh token")
		return
	}
	writeJSON(w, http.StatusOK, toTokenPairResponse(pair))
}
