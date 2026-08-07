package db

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type fakeStats struct {
	samples []PoolStats
	i       int
}

func (f *fakeStats) PoolStats() PoolStats {
	s := f.samples[f.i]
	f.i++
	return s
}

func saturated(emptyAcquires int64) PoolStats {
	return PoolStats{MaxConns: 25, AcquiredConns: 25, EmptyAcquireCount: emptyAcquires, Saturated: true}
}

func healthy() PoolStats {
	return PoolStats{MaxConns: 25, AcquiredConns: 3, Saturated: false}
}

func runSamples(t *testing.T, samples []PoolStats) []map[string]any {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	sampler := NewSaturationSampler(&fakeStats{samples: samples}, logger)

	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sampler.run(ctx, ticks)
		close(done)
	}()

	for range samples {
		ticks <- time.Now()
	}
	cancel()
	<-done

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	return records
}

func TestSaturationBelowThresholdIsSilent(t *testing.T) {
	got := runSamples(t, []PoolStats{saturated(1), saturated(2), saturated(3)})
	if len(got) != 0 {
		t.Errorf("got %d warnings, want 0: %v", len(got), got)
	}
}

func TestSaturationAtThresholdWarnsOnce(t *testing.T) {
	got := runSamples(t, []PoolStats{saturated(1), saturated(2), saturated(3), saturated(9)})
	if len(got) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(got), got)
	}
	if got[0]["msg"] != "db pool saturated" {
		t.Errorf("msg = %v, want %q", got[0]["msg"], "db pool saturated")
	}
	if got[0]["consecutive_saturated_samples"] != float64(4) {
		t.Errorf("consecutive_saturated_samples = %v, want 4", got[0]["consecutive_saturated_samples"])
	}
	if got[0]["empty_acquire_count_delta"] != float64(8) {
		t.Errorf("empty_acquire_count_delta = %v, want 8 (9 - episode base 1)", got[0]["empty_acquire_count_delta"])
	}
}

func TestSustainedSaturationDoesNotWarnPerSample(t *testing.T) {
	samples := make([]PoolStats, 12)
	for i := range samples {
		samples[i] = saturated(int64(i))
	}
	if got := runSamples(t, samples); len(got) != 1 {
		t.Errorf("got %d warnings over 12 saturated samples, want 1", len(got))
	}
}

func TestUnsaturatedSampleResetsCounter(t *testing.T) {
	got := runSamples(t, []PoolStats{
		saturated(1), saturated(2), saturated(3),
		healthy(),
		saturated(4), saturated(5), saturated(6),
	})
	if len(got) != 0 {
		t.Errorf("got %d warnings, want 0: an intervening healthy sample must reset the run", len(got))
	}
}

func TestSecondEpisodeWarnsAgain(t *testing.T) {
	got := runSamples(t, []PoolStats{
		saturated(1), saturated(2), saturated(3), saturated(4),
		healthy(),
		saturated(5), saturated(6), saturated(7), saturated(8),
	})
	if len(got) != 2 {
		t.Fatalf("got %d warnings, want 2 (one per sustained episode): %v", len(got), got)
	}
}
