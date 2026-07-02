package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	r := httpapi.NewRouter()

	w := testutil.DoRequest(r, "GET", "/api/health", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
