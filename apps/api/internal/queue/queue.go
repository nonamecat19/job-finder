package queue

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	TypeIngest      = "ingest"
	TypeMatch       = "match"
	TypeGenerate    = "generate"
	TypeEnrich      = "enrich"
	TypeSalaryInfer = "salary"
	TypeGhostScore  = "ghost"
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

	GenerationRunID *string `json:"generationRunId,omitempty"`

	IsRerun       bool     `json:"isRerun,omitempty"`
	RerunSections []string `json:"rerunSections,omitempty"`
}

func NewRedisClient(redisURL string) (redis.UniversalClient, error) {
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	u, err := url.Parse(redisURL)
	if err != nil {
		return nil, fmt.Errorf("queue: invalid REDIS_URL: %w", err)
	}
	port := u.Port()
	if port == "" {
		port = "6379"
	}
	opt := &redis.Options{Addr: u.Hostname() + ":" + port}
	if pw, ok := u.User.Password(); ok {
		opt.Password = pw
	}
	if u.Path != "" && u.Path != "/" {
		if db, err := strconv.Atoi(u.Path[1:]); err == nil {
			opt.DB = db
		}
	}
	return redis.NewClient(opt), nil
}
