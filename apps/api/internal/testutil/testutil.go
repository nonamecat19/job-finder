package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
)

// SetupRouter creates a chi mux with /api prefix and mounts the given handlers.
func SetupRouter(mounts ...func(chi.Router)) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(api chi.Router) {
		for _, mount := range mounts {
			mount(api)
		}
	})
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
