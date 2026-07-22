package httpapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/extauth"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakeExtAuthProvider struct {
	code       string
	exchangeOK bool
	refreshOK  bool
}

func (f *fakeExtAuthProvider) IssueBootstrapCode(ctx context.Context) (string, time.Time, error) {
	return f.code, time.Now().Add(5 * time.Minute), nil
}

func (f *fakeExtAuthProvider) ExchangeBootstrapCode(ctx context.Context, code string) (extauth.TokenPair, error) {
	if !f.exchangeOK || code != f.code {
		return extauth.TokenPair{}, extauth.ErrInvalidCode
	}
	return extauth.TokenPair{
		AccessToken:           "access-token",
		AccessTokenExpiresAt:  time.Now().Add(time.Hour),
		RefreshToken:          "refresh-token",
		RefreshTokenExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Scope:                 extauth.ScopeProfileRead,
	}, nil
}

func (f *fakeExtAuthProvider) RefreshTokens(ctx context.Context, refreshToken string) (extauth.TokenPair, error) {
	if !f.refreshOK {
		return extauth.TokenPair{}, extauth.ErrInvalidRefreshToken
	}
	return extauth.TokenPair{
		AccessToken:           "new-access-token",
		AccessTokenExpiresAt:  time.Now().Add(time.Hour),
		RefreshToken:          "new-refresh-token",
		RefreshTokenExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Scope:                 extauth.ScopeProfileRead,
	}, nil
}

func TestExtAuthBootstrap(t *testing.T) {
	h := &httpapi.ExtAuthHandler{Auth: &fakeExtAuthProvider{code: "abc123"}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/v1/ext/auth/bootstrap", nil, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]string
	testutil.ParseJSON(w, &out)
	if out["code"] != "abc123" {
		t.Errorf("code = %q, want %q", out["code"], "abc123")
	}
}

func TestExtAuthExchange_Success(t *testing.T) {
	h := &httpapi.ExtAuthHandler{Auth: &fakeExtAuthProvider{code: "abc123", exchangeOK: true}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/v1/ext/auth/exchange", map[string]any{"code": "abc123"}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]string
	testutil.ParseJSON(w, &out)
	if out["accessToken"] != "access-token" || out["refreshToken"] != "refresh-token" {
		t.Errorf("unexpected token pair: %+v", out)
	}
	if out["scope"] != extauth.ScopeProfileRead {
		t.Errorf("scope = %q, want %q", out["scope"], extauth.ScopeProfileRead)
	}
}

func TestExtAuthExchange_InvalidCode(t *testing.T) {
	h := &httpapi.ExtAuthHandler{Auth: &fakeExtAuthProvider{code: "abc123", exchangeOK: false}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/v1/ext/auth/exchange", map[string]any{"code": "wrong"}, nil)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExtAuthRefresh_Success(t *testing.T) {
	h := &httpapi.ExtAuthHandler{Auth: &fakeExtAuthProvider{refreshOK: true}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/v1/ext/auth/refresh", map[string]any{"refreshToken": "old"}, nil)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]string
	testutil.ParseJSON(w, &out)
	if out["accessToken"] != "new-access-token" {
		t.Errorf("accessToken = %q", out["accessToken"])
	}
}

func TestExtAuthRefresh_Invalid(t *testing.T) {
	h := &httpapi.ExtAuthHandler{Auth: &fakeExtAuthProvider{refreshOK: false}}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/v1/ext/auth/refresh", map[string]any{"refreshToken": "bad"}, nil)
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}
