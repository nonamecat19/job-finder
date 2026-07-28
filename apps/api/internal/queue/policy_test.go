package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/job-finder/api/internal/config"
)

func validConfig() *config.Config {
	return &config.Config{
		AIConcurrencyCloud:        3,
		AIConcurrencyLocal:        1,
		IngestConcurrency:         2,
		EnrichConcurrency:         1,
		AITaskTimeoutMatch:        5 * time.Minute,
		AITaskTimeoutGenerate:     15 * time.Minute,
		AITaskTimeoutSalary:       5 * time.Minute,
		AITaskTimeoutGhost:        5 * time.Minute,
		AITaskTimeoutEnrich:       10 * time.Minute,
		AITaskTimeoutIngest:       30 * time.Minute,
		ActivityHeartbeatInterval: 30 * time.Second,
		ActivityStaleAfter:        2 * time.Minute,
		ActivitySweepInterval:     1 * time.Minute,
		ActivityQueuedGrace:       30 * time.Minute,
	}
}

func TestPoliciesFromConfig_AllSixTaskTypes(t *testing.T) {
	policies, err := PoliciesFromConfig(validConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(policies) != 6 {
		t.Fatalf("expected 6 policies, got %d", len(policies))
	}
	want := map[string]string{
		TypeIngest:      "",
		TypeMatch:       "match",
		TypeGenerate:    "generation",
		TypeEnrich:      "",
		TypeSalaryInfer: "default",
		TypeGhostScore:  "ghost",
	}
	for _, p := range policies {
		key, ok := want[p.TaskType]
		if !ok {
			t.Fatalf("unexpected task type %q", p.TaskType)
		}
		if p.LLMTaskKey != key {
			t.Errorf("%s: expected LLMTaskKey %q, got %q", p.TaskType, key, p.LLMTaskKey)
		}
		delete(want, p.TaskType)
	}
	if len(want) != 0 {
		t.Fatalf("missing task types: %v", want)
	}
}

func TestPoliciesFromConfig_PoolSize(t *testing.T) {
	policies, err := PoliciesFromConfig(validConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range policies {
		if p.LLMTaskKey == "" {
			continue // non-LLM: local == hosted by construction
		}
		if got, want := p.PoolSize(), 3; got != want {
			t.Errorf("%s: PoolSize() = %d, want %d", p.TaskType, got, want)
		}
	}
}

func TestPoliciesFromConfig_RejectsLowConcurrency(t *testing.T) {
	cfg := validConfig()
	cfg.AIConcurrencyLocal = 0
	_, err := PoliciesFromConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "local concurrency") {
		t.Fatalf("expected local concurrency error, got %v", err)
	}
}

func TestPoliciesFromConfig_RejectsZeroHostedConcurrency(t *testing.T) {
	cfg := validConfig()
	cfg.AIConcurrencyCloud = 0
	_, err := PoliciesFromConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "hosted concurrency") {
		t.Fatalf("expected hosted concurrency error, got %v", err)
	}
}

func TestPoliciesFromConfig_RejectsZeroDuration(t *testing.T) {
	cfg := validConfig()
	cfg.AITaskTimeoutMatch = 0
	_, err := PoliciesFromConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "max duration") {
		t.Fatalf("expected max duration error, got %v", err)
	}
}

func TestPoliciesFromConfig_RejectsStaleAfterBelowTwiceHeartbeat(t *testing.T) {
	cfg := validConfig()
	cfg.ActivityStaleAfter = cfg.ActivityHeartbeatInterval // exactly 1x, not 2x
	_, err := PoliciesFromConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "ACTIVITY_STALE_AFTER") {
		t.Fatalf("expected stale-after error, got %v", err)
	}
}

func TestPoliciesFromConfig_RejectsStaleAfterPlusSweepAtOrOverFiveMinutes(t *testing.T) {
	cfg := validConfig()
	cfg.ActivityStaleAfter = 4 * time.Minute
	cfg.ActivitySweepInterval = 1 * time.Minute // sums to exactly 5m
	_, err := PoliciesFromConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "must be < 5m") {
		t.Fatalf("expected 5m bound error, got %v", err)
	}
}

func TestPoliciesFromConfig_AcceptsBoundaryUnderFiveMinutes(t *testing.T) {
	cfg := validConfig()
	cfg.ActivityStaleAfter = 3*time.Minute + 59*time.Second
	cfg.ActivitySweepInterval = 1 * time.Minute // sums to 4m59s, just under 5m
	if _, err := PoliciesFromConfig(cfg); err != nil {
		t.Fatalf("unexpected error at boundary: %v", err)
	}
}
