package db

// PoolStats is a point-in-time snapshot of connection-pool state, consumed by
// the readiness report and the saturation sampler (026-db-pool-capacity).
//
// EmptyAcquireCount and AcquireDurationMs are cumulative since process start,
// not gauges: one reading cannot say whether the pool is struggling right now,
// two consecutive readings can. Rate treatment belongs with a metrics system
// (research.md R6).
type PoolStats struct {
	MaxConns          int32 `json:"max_conns"`
	AcquiredConns     int32 `json:"acquired_conns"`
	IdleConns         int32 `json:"idle_conns"`
	TotalConns        int32 `json:"total_conns"`
	EmptyAcquireCount int64 `json:"empty_acquire_count"`
	AcquireDurationMs int64 `json:"acquire_duration_ms"`
	Saturated         bool  `json:"saturated"`
}

// PoolStats reports the pool's current state.
func (d *DB) PoolStats() PoolStats {
	s := d.Pool.Stat()
	return PoolStats{
		MaxConns:          s.MaxConns(),
		AcquiredConns:     s.AcquiredConns(),
		IdleConns:         s.IdleConns(),
		TotalConns:        s.TotalConns(),
		EmptyAcquireCount: s.EmptyAcquireCount(),
		AcquireDurationMs: s.AcquireDuration().Milliseconds(),
		Saturated:         s.AcquiredConns() >= s.MaxConns(),
	}
}
