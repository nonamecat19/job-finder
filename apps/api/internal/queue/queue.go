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
	TypeIngest   = "ingest"
	TypeMatch    = "match"
	TypeGenerate = "generate"
	TypeEnrich   = "enrich"
)

// Queue names each task type is routed to. These are deliberately separate
// asynq *queues* (not just task type names) so each can run on its own
// dedicated asynq.Server with its own Concurrency cap — mirroring the BullMQ
// setup where ingest/match/generate were three separate queues with
// concurrency 2/1/1 respectively (ingestion.processor.ts, matching.processor.ts,
// generation.processor.ts). A single asynq.Server's `Queues` map only
// controls priority weighting *within* one shared worker pool, not a hard
// per-queue concurrency ceiling, so one server can't reproduce "match never
// runs more than 1 at a time" on its own.
const (
	QueueIngest   = TypeIngest
	QueueMatch    = TypeMatch
	QueueGenerate = TypeGenerate
	QueueEnrich   = TypeEnrich
)

// IngestPayload mirrors IngestJobData. Exactly one of SearchID/SubscriptionID
// is set for a saved-search or subscription run; both nil means "scrape with
// an empty query" (e.g. a direct source test).
type IngestPayload struct {
	SearchID       *string `json:"searchId"`
	SubscriptionID *string `json:"subscriptionId,omitempty"`
	SourceKey      string  `json:"sourceKey"`
}

// MatchPayload mirrors MatchJobData.
type MatchPayload struct {
	JobID string `json:"jobId"`
}

// EnrichPayload carries the job to fetch full detail for.
type EnrichPayload struct {
	JobID string `json:"jobId"`
}

// GeneratePayload mirrors GenerateJobData.
type GeneratePayload struct {
	JobID     string  `json:"jobId"`
	Type      string  `json:"type"` // "resume" | "cover_letter"
	ProfileID *string `json:"profileId,omitempty"`
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
