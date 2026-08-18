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

func NewRouter(acquireTimeout time.Duration, mounts ...func(chi.Router)) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) //nolint:staticcheck
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
