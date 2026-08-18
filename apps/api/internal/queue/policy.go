package queue

import (
	"fmt"
	"time"

	"github.com/job-finder/api/internal/config"
)

type TaskPolicy struct {
	TaskType    string
	Queue       string
	Concurrency int
	MaxDuration time.Duration
	LLMTaskKey  string
}

// PoolSize returns the worker concurrency for this policy. Since 044 there is
// only one concurrency setting — the hosted/local split is gone along with
// Ollama.
func (p TaskPolicy) PoolSize() int {
	return p.Concurrency
}

func PoliciesFromConfig(cfg *config.Config) ([]TaskPolicy, error) {
	policies := []TaskPolicy{
		{
			TaskType:    TypeIngest,
			Queue:       QueueIngest,
			Concurrency: cfg.IngestConcurrency,
			MaxDuration: cfg.AITaskTimeoutIngest,
		},
		{
			TaskType:    TypeMatch,
			Queue:       QueueMatch,
			Concurrency: cfg.AIConcurrencyCloud,
			MaxDuration: cfg.AITaskTimeoutMatch,
			LLMTaskKey:  "match",
		},
		{
			TaskType:    TypeGenerate,
			Queue:       QueueGenerate,
			Concurrency: cfg.AIConcurrencyCloud,
			MaxDuration: cfg.AITaskTimeoutGenerate,
			LLMTaskKey:  "generation",
		},
		{
			TaskType:    TypeEnrich,
			Queue:       QueueEnrich,
			Concurrency: cfg.EnrichConcurrency,
			MaxDuration: cfg.AITaskTimeoutEnrich,
		},
		{
			TaskType:    TypeSalaryInfer,
			Queue:       QueueSalaryInfer,
			Concurrency: cfg.AIConcurrencyCloud,
			MaxDuration: cfg.AITaskTimeoutSalary,
			LLMTaskKey:  "salary",
		},
		{
			TaskType:    TypeGhostScore,
			Queue:       QueueGhostScore,
			Concurrency: cfg.AIConcurrencyCloud,
			MaxDuration: cfg.AITaskTimeoutGhost,
			LLMTaskKey:  "ghost",
		},
	}

	for _, p := range policies {
		if err := validatePolicy(p); err != nil {
			return nil, err
		}
	}
	if err := validateLiveness(cfg); err != nil {
		return nil, err
	}
	return policies, nil
}

func validatePolicy(p TaskPolicy) error {
	if p.Concurrency < 1 {
		return fmt.Errorf("queue: %s: concurrency must be >= 1, got %d", p.TaskType, p.Concurrency)
	}
	if p.MaxDuration <= 0 {
		return fmt.Errorf("queue: %s: max duration must be > 0, got %s", p.TaskType, p.MaxDuration)
	}
	return nil
}

func validateLiveness(cfg *config.Config) error {
	if cfg.ActivityHeartbeatInterval <= 0 {
		return fmt.Errorf("queue: ACTIVITY_HEARTBEAT_INTERVAL must be > 0, got %s", cfg.ActivityHeartbeatInterval)
	}
	if cfg.ActivityStaleAfter < 2*cfg.ActivityHeartbeatInterval {
		return fmt.Errorf("queue: ACTIVITY_STALE_AFTER (%s) must be >= 2x ACTIVITY_HEARTBEAT_INTERVAL (%s)", cfg.ActivityStaleAfter, cfg.ActivityHeartbeatInterval)
	}
	if cfg.ActivitySweepInterval <= 0 {
		return fmt.Errorf("queue: ACTIVITY_SWEEP_INTERVAL must be > 0, got %s", cfg.ActivitySweepInterval)
	}
	if cfg.ActivityStaleAfter+cfg.ActivitySweepInterval >= 5*time.Minute {
		return fmt.Errorf("queue: ACTIVITY_STALE_AFTER (%s) + ACTIVITY_SWEEP_INTERVAL (%s) must be < 5m", cfg.ActivityStaleAfter, cfg.ActivitySweepInterval)
	}
	if cfg.ActivityQueuedGrace <= 0 {
		return fmt.Errorf("queue: ACTIVITY_QUEUED_GRACE must be > 0, got %s", cfg.ActivityQueuedGrace)
	}
	return nil
}

// RateLimitRetryDelay was ported to events.RateLimitRetryDelay (T024,
// data-model.md § 6) now that retry delay is computed as a delay-queue TTL
// rather than an asynq RetryDelayFunc.
