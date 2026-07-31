package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/job-finder/api/internal/httpx"
)

var startTime = time.Now()

// NewRouter builds the API router. acquireTimeout bounds how long any request
// may wait on a database connection before failing with a capacity error
// (026-db-pool-capacity); pass 0 to leave requests unbounded.
func NewRouter(acquireTimeout time.Duration, mounts ...func(chi.Router)) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// middleware.RealIP is deprecated (spoofable via X-Forwarded-For/X-Real-IP
	// when the caller controls those headers directly). Acceptable here: this
	// API is only ever reached through the project's own dashboard/proxy, IP
	// is used for request logging only (never for auth or rate-limit
	// decisions), and there is no untrusted public ingress in front of it.
	r.Use(middleware.RealIP) //nolint:staticcheck // SA1019: see rationale above
	r.Use(requestLogger)
	r.Use(middleware.Recoverer)
	if acquireTimeout > 0 {
		r.Use(acquireDeadline(acquireTimeout))
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
	}))

	mountAll := func(router chi.Router) {
		router.Get("/health", healthHandler)
		for _, mount := range mounts {
			mount(router)
		}
	}

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not found: "+r.URL.Path)
	})

	r.Route("/api", mountAll)
	r.Route("/api/v1", mountAll)

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"uptime": time.Since(startTime).Seconds(),
	})
}
