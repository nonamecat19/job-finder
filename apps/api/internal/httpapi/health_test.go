package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(ctx context.Context) error { return f.err }

func doHealthRequest(r http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/health/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeReadiness(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestHealthReady_AllOK(t *testing.T) {
	h := &httpapi.HealthHandler{
		Postgres: fakePinger{},
		Redis:    fakePinger{},
		Minio:    fakePinger{},
	}
	r := testutil.SetupRouter(h.Mount)

	w := doHealthRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeReadiness(t, w)
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("ok = %v, want true", body["ok"])
	}
	checks := body["checks"].(map[string]any)
	for _, dep := range []string{"postgres", "redis", "minio"} {
		c := checks[dep].(map[string]any)
		if c["status"] != "ok" {
			t.Errorf("%s status = %v, want ok", dep, c["status"])
		}
	}
}

func TestHealthReady_OneDependencyDown(t *testing.T) {
	h := &httpapi.HealthHandler{
		Postgres: fakePinger{err: errors.New("dial tcp: connection refused")},
		Redis:    fakePinger{},
		Minio:    fakePinger{},
	}
	r := testutil.SetupRouter(h.Mount)

	w := doHealthRequest(r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	body := decodeReadiness(t, w)
	if ok, _ := body["ok"].(bool); ok {
		t.Fatalf("ok = %v, want false", body["ok"])
	}
	checks := body["checks"].(map[string]any)
	pg := checks["postgres"].(map[string]any)
	if pg["status"] != "error" {
		t.Errorf("postgres status = %v, want error", pg["status"])
	}
	if pg["error"] == "" || pg["error"] == nil {
		t.Errorf("postgres error message missing")
	}
	redisCheck := checks["redis"].(map[string]any)
	if redisCheck["status"] != "ok" {
		t.Errorf("redis status = %v, want ok (unaffected by postgres failure)", redisCheck["status"])
	}
}

func TestHealthReady_MinioDisabled(t *testing.T) {
	h := &httpapi.HealthHandler{
		Postgres: fakePinger{},
		Redis:    fakePinger{},
		Minio:    nil,
	}
	r := testutil.SetupRouter(h.Mount)

	w := doHealthRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (minio disabled shouldn't fail readiness)", w.Code)
	}
	body := decodeReadiness(t, w)
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("ok = %v, want true", body["ok"])
	}
	checks := body["checks"].(map[string]any)
	minioCheck := checks["minio"].(map[string]any)
	if minioCheck["status"] != "disabled" {
		t.Errorf("minio status = %v, want disabled", minioCheck["status"])
	}
}
