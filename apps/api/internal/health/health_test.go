package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/health"
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
	h := &health.HealthHandler{
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
	h := &health.HealthHandler{
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
	h := &health.HealthHandler{
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

type fakePoolStatter struct{ stats db.PoolStats }

func (f fakePoolStatter) PoolStats() db.PoolStats { return f.stats }

func TestHealthReady_PoolBlockReported(t *testing.T) {
	h := &health.HealthHandler{
		Postgres: fakePinger{},
		Redis:    fakePinger{},
		Pool: fakePoolStatter{stats: db.PoolStats{
			MaxConns:          25,
			AcquiredConns:     4,
			IdleConns:         6,
			TotalConns:        10,
			EmptyAcquireCount: 3,
			AcquireDurationMs: 120,
		}},
	}

	w := doHealthRequest(testutil.SetupRouter(h.Mount))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	pool, ok := decodeReadiness(t, w)["pool"].(map[string]any)
	if !ok {
		t.Fatalf("pool block missing from %s", w.Body.String())
	}
	for field, want := range map[string]float64{
		"max_conns": 25, "acquired_conns": 4, "idle_conns": 6,
		"total_conns": 10, "empty_acquire_count": 3, "acquire_duration_ms": 120,
	} {
		if pool[field] != want {
			t.Errorf("pool.%s = %v, want %v", field, pool[field], want)
		}
	}
	if pool["saturated"] != false {
		t.Errorf("pool.saturated = %v, want false", pool["saturated"])
	}
}

func TestHealthReady_PoolOmittedWhenNil(t *testing.T) {
	h := &health.HealthHandler{Postgres: fakePinger{}, Redis: fakePinger{}}

	w := doHealthRequest(testutil.SetupRouter(h.Mount))
	// Omitted, not zero-valued: a zero block is indistinguishable from a
	// genuinely idle pool and would misreport max_conns as 0.
	if _, present := decodeReadiness(t, w)["pool"]; present {
		t.Errorf("pool key present with a nil Pool: %s", w.Body.String())
	}
}

func TestHealthReady_SaturationDoesNotAffectOK(t *testing.T) {
	h := &health.HealthHandler{
		Postgres: fakePinger{},
		Redis:    fakePinger{},
		Pool: fakePoolStatter{stats: db.PoolStats{
			MaxConns: 25, AcquiredConns: 25, TotalConns: 25, Saturated: true,
		}},
	}

	w := doHealthRequest(testutil.SetupRouter(h.Mount))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: a saturated pool is still serving", w.Code)
	}
	body := decodeReadiness(t, w)
	if ok, _ := body["ok"].(bool); !ok {
		t.Errorf("ok = %v, want true: saturation is a capacity signal, not unreadiness", body["ok"])
	}
	if pool := body["pool"].(map[string]any); pool["saturated"] != true {
		t.Errorf("pool.saturated = %v, want true", pool["saturated"])
	}
}
