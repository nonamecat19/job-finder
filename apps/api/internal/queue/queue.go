// Package queue defines the asynq task types and payloads shared by the
// ingestion/matching/generation handlers, mirroring common/queues.ts
// (BullMQ's QUEUE_INGEST/QUEUE_MATCH/QUEUE_GENERATE + job data shapes).
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

// Queue names each task type is routed to. These are deliberately separate
// asynq *queues* (not just task type names) so each can run on its own
// dedicated asynq.Server, sized from that task's TaskPolicy and gated by
// provider class (019-ai-job-throughput, internal/queue/policy.go +
// middleware.go) — hosted providers run several AI items at once per task
// type, local Ollama stays at one. A single asynq.Server's `Queues` map only
// controls priority weighting *within* one shared worker pool, not a hard
// per-queue concurrency ceiling, so each task type still needs its own server.
const (
	QueueIngest      = TypeIngest
	QueueMatch       = TypeMatch
	QueueGenerate    = TypeGenerate
	QueueEnrich      = TypeEnrich
	QueueSalaryInfer = TypeSalaryInfer
	QueueGhostScore  = TypeGhostScore
)

// IngestMaxRetry is the asynq MaxRetry for ingest tasks: 3 deliveries total,
// spaced by asynq's default exponential backoff.
//
// Ingest used to run with MaxRetry(0), inherited from the BullMQ setup's
// `{ attempts: 1 }`. Scraping is the least reliable step in the pipeline and
// the runs are hours apart, so a single 503 or timeout silently cost that
// source its whole cron window. Errors that retrying cannot fix are wrapped
// in asynq.SkipRetry by the handler instead (see ingestion.permanent), so the
// retries only ever spend themselves on transient failures.
const IngestMaxRetry = 2

// IngestPayload mirrors IngestJobData. Exactly one of SearchID/SubscriptionID
// is set for a saved-search or subscription run; both nil means "scrape with
// an empty query" (e.g. a direct source test).
type IngestPayload struct {
	SearchID       *string `json:"searchId"`
	SubscriptionID *string `json:"subscriptionId,omitempty"`
	SourceKey      string  `json:"sourceKey"`
	ActivityID     *string `json:"activityId,omitempty"`
}

// MatchPayload mirrors MatchJobData.
type MatchPayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

// EnrichPayload carries the job to fetch full detail for.
type EnrichPayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

// SalaryInferPayload carries the job to infer salary for.
type SalaryInferPayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

// GhostScorePayload carries the job to run the ghost-job detector (005)
// against. Triggered by ingestion and by the manual POST
// /api/jobs/{id}/ghost-score endpoint only — never on a schedule (FR-014).
type GhostScorePayload struct {
	JobID      string  `json:"jobId"`
	ActivityID *string `json:"activityId,omitempty"`
}

// GeneratePayload mirrors GenerateJobData.
type GeneratePayload struct {
	JobID      string  `json:"jobId"`
	Type       string  `json:"type"` // "resume" | "cover_letter"
	ProfileID  *string `json:"profileId,omitempty"`
	ActivityID *string `json:"activityId,omitempty"`
}

// RedisOpt parses a redis:// URL into asynq's client/server connection
// options, mirroring redisConnection() in queues.ts.
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
