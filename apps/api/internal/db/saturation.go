package db

import (
	"context"
	"log/slog"
	"time"
)

const (
	// saturationSampleInterval matches the ACTIVITY_HEARTBEAT_INTERVAL
	// convention; not exposed as a setting until an operator needs it.
	saturationSampleInterval = 30 * time.Second
	// saturationWarnThreshold is how many consecutive saturated samples
	// constitute exhaustion rather than a burst: 4 at 30s is ~2 minutes. A
	// single saturated sample is normal and must not log (data-model.md §4).
	saturationWarnThreshold = 4
)

// StatsSource is the sampler's view of the pool, so tests can drive the state
// machine without a database.
type StatsSource interface {
	PoolStats() PoolStats
}

// SaturationSampler warns when the pool has been continuously saturated long
// enough to be capacity exhaustion rather than an ordinary burst. It logs once
// per sustained episode, not once per sample.
type SaturationSampler struct {
	stats     StatsSource
	logger    *slog.Logger
	threshold int

	consecutive int
	warned      bool
	episodeBase int64
}

// NewSaturationSampler builds a sampler over src. A nil logger uses the default.
func NewSaturationSampler(src StatsSource, logger *slog.Logger) *SaturationSampler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SaturationSampler{stats: src, logger: logger, threshold: saturationWarnThreshold}
}

// Run samples until ctx is cancelled.
func (s *SaturationSampler) Run(ctx context.Context) {
	ticker := time.NewTicker(saturationSampleInterval)
	defer ticker.Stop()
	s.run(ctx, ticker.C)
}

// run is Run with an injectable tick source.
func (s *SaturationSampler) run(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			s.sample()
		}
	}
}

// sample folds one observation into the episode state machine, emitting a
// warning on the sample that first crosses the threshold.
func (s *SaturationSampler) sample() {
	st := s.stats.PoolStats()
	if !st.Saturated {
		s.consecutive = 0
		s.warned = false
		return
	}
	if s.consecutive == 0 {
		s.episodeBase = st.EmptyAcquireCount
	}
	s.consecutive++
	if s.consecutive < s.threshold || s.warned {
		return
	}
	s.warned = true
	s.logger.Warn("db pool saturated",
		"max_conns", st.MaxConns,
		"acquired_conns", st.AcquiredConns,
		// Growth over this episode, not since process start: the cumulative
		// counter alone cannot distinguish a current stall from an old one.
		"empty_acquire_count_delta", st.EmptyAcquireCount-s.episodeBase,
		"consecutive_saturated_samples", s.consecutive,
	)
}
