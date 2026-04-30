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
)

// IngestPayload mirrors IngestJobData.
type IngestPayload struct {
	SearchID  *string `json:"searchId"`
	SourceKey string  `json:"sourceKey"`
}

// MatchPayload mirrors MatchJobData.
type MatchPayload struct {
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
