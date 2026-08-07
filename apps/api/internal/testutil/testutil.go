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

var ErrFake = errors.New("boom")

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

func ParseJSON(w *httptest.ResponseRecorder, v any) {
	json.NewDecoder(w.Body).Decode(v)
}
