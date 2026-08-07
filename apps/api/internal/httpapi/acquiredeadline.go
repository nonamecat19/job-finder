package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/httpx"
)

const capacityExhaustedMessage = "database connection capacity exhausted; the connection pool is saturated"

func acquireDeadline(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if deadline, ok := r.Context().Deadline(); ok && time.Until(deadline) <= timeout {
				next.ServeHTTP(w, r)
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			tracked := &writeTracker{ResponseWriter: w}
			next.ServeHTTP(tracked, r.WithContext(ctx))

			if !tracked.wrote && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				httpx.WriteAppError(w, apperr.New(apperr.KindUnavailable, capacityExhaustedMessage))
			}
		})
	}
}

type writeTracker struct {
	http.ResponseWriter
	wrote bool
}

func (w *writeTracker) WriteHeader(status int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *writeTracker) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}
