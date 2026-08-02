package queue

import (
	"fmt"
	"time"

	"github.com/job-finder/api/internal/config"
)

// TaskPolicy is the resolved concurrency/deadline configuration for one AI
// task type, built once at startup from config.Config (019-ai-job-throughput).
// Non-LLM tasks (ingest, enrich) have an empty LLMTaskKey and use
// LocalConcurrency as their single fixed pool size; HostedConcurrency is
// unused for them.
type TaskPolicy struct {
	TaskType          string
	Queue             string
	LocalConcurrency  int
	HostedConcurrency int
	MaxDuration       time.Duration
	// LLMTaskKey is the gateway task key ("match", "generation", "default",
	// "ghost") used to resolve the live provider class. Empty for non-LLM tasks.
	LLMTaskKey string
}

// PoolSize is the asynq worker pool size: the maximum of the two
// concurrency levels, since asynq.Config.Concurrency is fixed at server
// construction while the admission gate enforces the applicable limit per
// task at runtime (research.md R3).
func (p TaskPolicy) PoolSize() int {
	if p.HostedConcurrency > p.LocalConcurrency {
		return p.HostedConcurrency
	}
	return p.LocalConcurrency
}

// PoliciesFromConfig builds one TaskPolicy per AI task type from cfg,
// covering all six task types (data-model.md §2).
func PoliciesFromConfig(cfg *config.Config) ([]TaskPolicy, error) {
	policies := []TaskPolicy{
		{
			TaskType:          TypeIngest,
			Queue:             QueueIngest,
			LocalConcurrency:  cfg.IngestConcurrency,
			HostedConcurrency: cfg.IngestConcurrency,
			MaxDuration:       cfg.AITaskTimeoutIngest,
		},
		{
			TaskType:          TypeMatch,
			Queue:             QueueMatch,
			LocalConcurrency:  cfg.AIConcurrencyLocal,
			HostedConcurrency: cfg.AIConcurrencyCloud,
			MaxDuration:       cfg.AITaskTimeoutMatch,
			LLMTaskKey:        "match",
		},
		{
			TaskType:          TypeGenerate,
			Queue:             QueueGenerate,
			LocalConcurrency:  cfg.AIConcurrencyLocal,
			HostedConcurrency: cfg.AIConcurrencyCloud,
			MaxDuration:       cfg.AITaskTimeoutGenerate,
			LLMTaskKey:        "generation",
		},
		{
			TaskType:          TypeEnrich,
			Queue:             QueueEnrich,
			LocalConcurrency:  cfg.EnrichConcurrency,
			HostedConcurrency: cfg.EnrichConcurrency,
			MaxDuration:       cfg.AITaskTimeoutEnrich,
		},
		{
			TaskType:          TypeSalaryInfer,
			Queue:             QueueSalaryInfer,
			LocalConcurrency:  cfg.AIConcurrencyLocal,
			HostedConcurrency: cfg.AIConcurrencyCloud,
			MaxDuration:       cfg.AITaskTimeoutSalary,
			LLMTaskKey:        "default",
		},
		{
			TaskType:          TypeGhostScore,
			Queue:             QueueGhostScore,
			LocalConcurrency:  cfg.AIConcurrencyLocal,
			HostedConcurrency: cfg.AIConcurrencyCloud,
			MaxDuration:       cfg.AITaskTimeoutGhost,
			LLMTaskKey:        "ghost",
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
	if p.LocalConcurrency < 1 {
		return fmt.Errorf("queue: %s: local concurrency must be >= 1, got %d", p.TaskType, p.LocalConcurrency)
	}
	if p.HostedConcurrency < 1 {
		return fmt.Errorf("queue: %s: hosted concurrency must be >= 1, got %d", p.TaskType, p.HostedConcurrency)
	}
	if p.MaxDuration <= 0 {
		return fmt.Errorf("queue: %s: max duration must be > 0, got %s", p.TaskType, p.MaxDuration)
	}
	return nil
}

// validateLiveness enforces the sweeper's bounds (FR-009/SC-005):
// ACTIVITY_STALE_AFTER must be at least 2x ACTIVITY_HEARTBEAT_INTERVAL, and
// ACTIVITY_STALE_AFTER+ACTIVITY_SWEEP_INTERVAL must stay under 5 minutes.
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
