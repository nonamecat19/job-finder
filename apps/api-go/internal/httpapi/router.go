// Package httpapi wires chi handlers mirroring the NestJS controllers.
// All routes are mounted under /api, matching `app.setGlobalPrefix('api')`.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

var startTime = time.Now()

// NewRouter builds the top-level chi router. Additional route groups are
// mounted onto the returned router by later phases (searches, jobs, ...).
func NewRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
	}))

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", healthHandler)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"uptime": time.Since(startTime).Seconds(),
	})
}

// writeJSON is the shared JSON response helper used by every handler package.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response failed", "error", err)
	}
}

// writeError writes a JSON error body {"message": "..."} — same shape Nest's
// default exception filter produces, which the dashboard already parses.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}

// decodeJSON reads and decodes a JSON request body into dst.
func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
