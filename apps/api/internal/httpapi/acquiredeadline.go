package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/httpx"
)

// capacityExhaustedMessage names the condition explicitly. A starved request
// that returns a bare 500 (or hangs) is indistinguishable from a slow query,
// which is the spec's central complaint (026-db-pool-capacity, FR-008a).
const capacityExhaustedMessage = "database connection capacity exhausted; the connection pool is saturated"

// acquireDeadline bounds how long an interactive request may spend waiting for
// a database connection. pgxpool has no AcquireTimeout — Acquire(ctx) blocks
// until ctx is done — so the bound has to come from the request context
// (research.md R5). Background workers are unaffected; they keep their longer
// TaskPolicy.MaxDuration deadlines.
//
// A handler that returns having written nothing with the derived deadline
// expired was starved, and gets a distinguishable capacity error rather than
// a bare 500.
func acquireDeadline(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Never extend a caller that is already on a shorter clock: the
			// tighter of the two deadlines is the one that matters.
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

// writeTracker records whether the handler produced any response, so the
// middleware never writes a second set of headers over a handler that already
// answered.
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
