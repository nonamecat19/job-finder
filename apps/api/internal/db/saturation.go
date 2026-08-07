package db

import (
	"context"
	"log/slog"
	"time"
)

const (
	saturationSampleInterval = 30 * time.Second
	saturationWarnThreshold  = 4
)

type StatsSource interface {
	PoolStats() PoolStats
}

type SaturationSampler struct {
	stats     StatsSource
	logger    *slog.Logger
	threshold int

	consecutive int
	warned      bool
	episodeBase int64
}

func NewSaturationSampler(src StatsSource, logger *slog.Logger) *SaturationSampler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SaturationSampler{stats: src, logger: logger, threshold: saturationWarnThreshold}
}

func (s *SaturationSampler) Run(ctx context.Context) {
	ticker := time.NewTicker(saturationSampleInterval)
	defer ticker.Stop()
	s.run(ctx, ticker.C)
}

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
		"empty_acquire_count_delta", st.EmptyAcquireCount-s.episodeBase,
		"consecutive_saturated_samples", s.consecutive,
	)
}
