package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// readinessTimeout bounds each individual dependency ping so a hung
// dependency cannot hang the whole /health/ready response (spec FR-004).
const readinessTimeout = 2 * time.Second

// Pinger is the structural interface HealthHandler needs from each
// dependency client. *pgxpool.Pool, redis.UniversalClient, and *minio.Client
// (via a small adapter) all satisfy this with their existing Ping/BucketExists
// methods — see cmd/server/compose.go for the concrete wiring.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves liveness/readiness. Postgres and Redis are always
// checked; Minio is optional — Minio == nil means uploads are disabled
// (mirrors internal/storage's "MinioEndpoint unset" convention) and is
// reported as "disabled" rather than failing readiness.
type HealthHandler struct {
	Postgres Pinger
	Redis    Pinger
	Minio    Pinger // nil if MinIO is not configured
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

	status := http.StatusOK
	if !report.OK {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}
