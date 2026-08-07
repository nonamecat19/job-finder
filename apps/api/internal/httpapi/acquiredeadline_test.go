package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/httpx"
)

func serveWithDeadline(t *testing.T, timeout time.Duration, req *http.Request, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	acquireDeadline(timeout)(h).ServeHTTP(rec, req)
	return rec
}

func TestAcquireDeadlineReportsCapacityExhaustion(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)

	rec := serveWithDeadline(t, 20*time.Millisecond, req, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body httpx.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if body.Code != apperr.KindUnavailable {
		t.Errorf("code = %q, want %q", body.Code, apperr.KindUnavailable)
	}
	if body.Message != capacityExhaustedMessage {
		t.Errorf("message = %q, want %q", body.Message, capacityExhaustedMessage)
	}
}

func TestAcquireDeadlineLeavesFastHandlerAlone(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)

	rec := serveWithDeadline(t, time.Second, req, func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"result": "fine"})
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"result\":\"fine\"}\n" {
		t.Errorf("body = %q, want the handler's own response", got)
	}
}

func TestAcquireDeadlineDoesNotExtendAShorterContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil).WithContext(ctx)

	var observed time.Duration
	start := time.Now()
	rec := serveWithDeadline(t, time.Hour, req, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		observed = time.Since(start)
		w.WriteHeader(http.StatusGatewayTimeout)
	})

	if observed > time.Second {
		t.Errorf("handler ran for %s; the caller's shorter deadline was replaced by the middleware's", observed)
	}
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}
}

func TestAcquireDeadlineKeepsHandlerResponseAfterExpiry(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)

	rec := serveWithDeadline(t, 10*time.Millisecond, req, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		httpx.WriteError(w, http.StatusNotFound, "not found")
	})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404: the middleware must not overwrite a handler that answered", rec.Code)
	}
}
