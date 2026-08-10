package queue

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/hibiken/asynq"
)

const (
	TypeIngest      = "ingest"
	TypeMatch       = "match"
	TypeGenerate    = "generate"
	TypeEnrich      = "enrich"
	TypeSalaryInfer = "salary:infer"
	TypeGhostScore  = "ghost:score"
)

const (
	QueueIngest      = TypeIngest
	QueueMatch       = TypeMatch
	QueueGenerate    = TypeGenerate
	QueueEnrich      = TypeEnrich
	QueueSalaryInfer = TypeSalaryInfer
	QueueGhostScore  = TypeGhostScore
)

const IngestMaxRetry = 2

type IngestPayload struct {
	SearchID       *string `json:"searchId"`
	SubscriptionID *string `json:"subscriptionId,omitempty"`
	SourceKey      string  `json:"sourceKey"`
	ActivityID     *string `json:"activityId,omitempty"`
}

type MatchPayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

type EnrichPayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

type SalaryInferPayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

type GhostScorePayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

type GeneratePayload struct {
	JobID      string  `json:"jobId"`
	Type       string  `json:"type"`
	ProfileID  *string `json:"profileId,omitempty"`
	ActivityID *string `json:"activityId,omitempty"`
	// GenerationRunID routes this task to the 042 workspace pipeline
	// (application.Service.StartRun) instead of the legacy merged-resume path.
	// Wire-nullable, so every caller written before this feature is unaffected.
	GenerationRunID *string `json:"generationRunId,omitempty"`
	// IsRerun routes a GenerationRunID task to application.Service.RerunRun
	// instead of StartRun (T076, `POST /v1/generations/{runId}/rerun`).
	// RerunSections names the sections to replace in place; empty means the
	// whole run, per rest-api.md's "omit for the whole run".
	IsRerun       bool     `json:"isRerun,omitempty"`
	RerunSections []string `json:"rerunSections,omitempty"`
}

func RedisOpt(redisURL string) (asynq.RedisClientOpt, error) {
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	u, err := url.Parse(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("queue: invalid REDIS_URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		port = "6379"
	}
	opt := asynq.RedisClientOpt{Addr: u.Hostname() + ":" + port}
	if pw, ok := u.User.Password(); ok {
		opt.Password = pw
	}
	if u.Path != "" && u.Path != "/" {
		if db, err := strconv.Atoi(u.Path[1:]); err == nil {
			opt.DB = db
		}
	}
	return opt, nil
}
