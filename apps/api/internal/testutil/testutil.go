package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
)

// ErrFake is the stand-in failure for handler tests that need a provider to
// return *some* error — the ones asserting a 500, where the specific error
// value is irrelevant. It lives here rather than in one handler's test file so
// that splitting httpapi into per-domain packages does not strand the tests
// that use it, or force a copy of the same three lines into every new package.
//
// Handlers that map a *particular* sentinel to a non-500 status must keep
// using that sentinel (see companyintel's application.ErrNoCompany → 422);
// this is only for the generic path.
var ErrFake = errors.New("boom")

// SetupRouter mirrors production's NewRouter (httpapi/router.go): it mounts
// the given handlers under both /api and /api/v1, since /api/v1 is a more
// specific chi route than /api and shadows it for any /api/v1/* request.
// A handler that hardcodes a "/v1/..." prefix in its own Mount pattern would
// only be reachable at /api/v1/v1/... under this topology — mirroring it
// here is what catches that class of bug instead of masking it.
func SetupRouter(mounts ...func(chi.Router)) *chi.Mux {
	r := chi.NewRouter()
	mountAll := func(api chi.Router) {
		for _, mount := range mounts {
			mount(api)
		}
	}
	r.Route("/api", mountAll)
	r.Route("/api/v1", mountAll)
	return r
}

// DoRequest creates an httptest request, sets optional chi URL params,
// serves it against the router, and returns the recorder.
func DoRequest(r chi.Router, method, url string, body io.Reader, params map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, body)
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// DoRequestJSON marshals body as JSON, sets Content-Type, and serves the request.
func DoRequestJSON(r chi.Router, method, url string, body any, params map[string]string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, url, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ParseJSON decodes the response body into v.
func ParseJSON(w *httptest.ResponseRecorder, v any) {
	json.NewDecoder(w.Body).Decode(v)
}
