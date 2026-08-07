package health

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/httpx"
)

const readinessTimeout = 2 * time.Second

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	Postgres Pinger
	Redis    Pinger
	Minio    Pinger

	Pool PoolStatter
}

type PoolStatter interface {
	PoolStats() db.PoolStats
}

func (h *HealthHandler) Mount(r chi.Router) {
	r.Get("/health/ready", h.ready)
}

type depCheck struct {
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
}

type readinessReport struct {
	OK     bool                `json:"ok"`
	Checks map[string]depCheck `json:"checks"`
	Pool   *db.PoolStats       `json:"pool,omitempty"`
}

func (h *HealthHandler) ready(w http.ResponseWriter, r *http.Request) {
	report := readinessReport{OK: true, Checks: map[string]depCheck{}}

	checkOne := func(name string, p Pinger) {
		if p == nil {
			report.Checks[name] = depCheck{Status: "disabled"}
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		start := time.Now()
		err := p.Ping(ctx)
		latency := time.Since(start).Milliseconds()
		if err != nil {
			report.OK = false
			report.Checks[name] = depCheck{Status: "error", Error: err.Error(), LatencyMs: latency}
			return
		}
		report.Checks[name] = depCheck{Status: "ok", LatencyMs: latency}
	}

	checkOne("postgres", h.Postgres)
	checkOne("redis", h.Redis)
	checkOne("minio", h.Minio)

	if h.Pool != nil {
		stats := h.Pool.PoolStats()
		report.Pool = &stats
	}

	status := http.StatusOK
	if !report.OK {
		status = http.StatusServiceUnavailable
	}
	httpx.WriteJSON(w, status, report)
}
