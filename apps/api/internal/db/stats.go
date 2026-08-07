package db

type PoolStats struct {
	MaxConns          int32 `json:"max_conns"`
	AcquiredConns     int32 `json:"acquired_conns"`
	IdleConns         int32 `json:"idle_conns"`
	TotalConns        int32 `json:"total_conns"`
	EmptyAcquireCount int64 `json:"empty_acquire_count"`
	AcquireDurationMs int64 `json:"acquire_duration_ms"`
	Saturated         bool  `json:"saturated"`
}

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
