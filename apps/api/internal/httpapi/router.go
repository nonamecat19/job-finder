// Package httpapi wires chi handlers mirroring the NestJS controllers.
// All routes are mounted under /api, matching `app.setGlobalPrefix('api')`.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

var startTime = time.Now()

// statusRecorder wraps http.ResponseWriter to capture the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// loggerMiddleware is a structured logging replacement for chi's middleware.Logger.
// It uses slog and logs 5xx responses at Error level (with request details), 4xx at
// Warn, and everything else at Info.
func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)

		attrs := []slog.Attr{
			slog.Int("status", sr.status),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
			slog.Duration("duration", time.Since(start)),
		}
		if reqID := middleware.GetReqID(r.Context()); reqID != "" {
			attrs = append(attrs, slog.String("request_id", reqID))
		}

		slog.LogAttrs(context.Background(), logLevel(sr.status), "request", attrs...)
	})
}

func logLevel(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// NewRouter builds the top-level chi router. Each mount function registers
// its routes onto the /api subrouter — one per controller-equivalent
// (sources, searches, jobs, profiles, applications, documents, ...).
func NewRouter(mounts ...func(chi.Router)) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(loggerMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: false,
	}))

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", healthHandler)
		for _, mount := range mounts {
			mount(api)
		}
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
// For 5xx responses it also logs the error via slog so it's visible in server
// logs even when the chi logger middleware doesn't capture the response body.
func writeError(w http.ResponseWriter, status int, message string) {
	if status >= 500 {
		slog.Error("handler error", "status", status, "message", message)
	}
	writeJSON(w, status, map[string]string{"message": message})
}

// decodeJSON reads and decodes a JSON request body into dst.
func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// reencode round-trips a generic decoded value (map[string]any/[]any/...)
// through JSON into a concrete type T. Used when a handler first decodes the
// body into a map (to distinguish "field absent" from "field null"), then
// needs one sub-field as a typed struct.
func reencode[T any](v any) T {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, &out)
	return out
}
