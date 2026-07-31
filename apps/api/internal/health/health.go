package health

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/httpx"
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

	// Pool reports connection-pool capacity (026-db-pool-capacity). Nil-able:
	// when nil the `pool` key is omitted entirely, since a zero-valued block
	// is indistinguishable from a genuinely idle pool and would misreport
	// max_conns as 0.
	Pool PoolStatter
}

// PoolStatter is implemented by the Postgres pool only. Redis and MinIO have
// no equivalent, so this is deliberately not folded into Pinger — widening
// Pinger would force meaningless implementations on the other two.
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

	// Saturation deliberately does not affect ok: a saturated pool is still
	// serving, and flipping readiness to false would pull the process out of
	// rotation for a load condition, making it worse (contracts/readiness.md).
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
