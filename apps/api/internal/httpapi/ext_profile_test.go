package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/extauth"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakeExtProfileProvider struct {
	profile dto.ExtProfileDto
	err     error
}

func (f *fakeExtProfileProvider) ExtProfile(ctx context.Context) (dto.ExtProfileDto, error) {
	return f.profile, f.err
}

func doAuthedRequest(r http.Handler, method, url, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExtProfile_Success(t *testing.T) {
	signer := extauth.NewSigner([]byte("test-secret-32-bytes-of-padding"))
	token, _, err := signer.Issue(extauth.Subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	h := &httpapi.ExtProfileHandler{
		Profiles: &fakeExtProfileProvider{profile: dto.ExtProfileDto{FullName: "Jane Doe", Email: "jane@example.com"}},
		Verifier: signer,
	}
	r := testutil.SetupRouter(h.Mount)

	w := doAuthedRequest(r, "GET", "/api/v1/ext/profile", token)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.ExtProfileDto
	testutil.ParseJSON(w, &out)
	if out.FullName != "Jane Doe" {
		t.Errorf("FullName = %q", out.FullName)
	}
}

func TestExtProfile_MissingToken(t *testing.T) {
	signer := extauth.NewSigner([]byte("test-secret-32-bytes-of-padding"))
	h := &httpapi.ExtProfileHandler{Profiles: &fakeExtProfileProvider{}, Verifier: signer}
	r := testutil.SetupRouter(h.Mount)

	w := doAuthedRequest(r, "GET", "/api/v1/ext/profile", "")
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestExtProfile_InvalidToken(t *testing.T) {
	signer := extauth.NewSigner([]byte("test-secret-32-bytes-of-padding"))
	h := &httpapi.ExtProfileHandler{Profiles: &fakeExtProfileProvider{}, Verifier: signer}
	r := testutil.SetupRouter(h.Mount)

	w := doAuthedRequest(r, "GET", "/api/v1/ext/profile", "garbage.token.value")
	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestExtProfile_WrongSecretToken(t *testing.T) {
	otherSigner := extauth.NewSigner([]byte("a-completely-different-32-bytes"))
	token, _, err := otherSigner.Issue(extauth.Subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	signer := extauth.NewSigner([]byte("test-secret-32-bytes-of-padding"))
	h := &httpapi.ExtProfileHandler{Profiles: &fakeExtProfileProvider{}, Verifier: signer}
	r := testutil.SetupRouter(h.Mount)

	w := doAuthedRequest(r, "GET", "/api/v1/ext/profile", token)
	if w.Code != 401 {
		t.Fatalf("expected 401 for a token signed with a different secret, got %d", w.Code)
	}
}
