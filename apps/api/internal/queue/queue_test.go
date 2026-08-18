package queue

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestQueueTypes(t *testing.T) {
	tests := []struct {
		name      string
		queueType string
		expected  string
	}{
		{"ingest queue", TypeIngest, "ingest"},
		{"match queue", TypeMatch, "match"},
		{"generate queue", TypeGenerate, "generate"},
		{"enrich queue", TypeEnrich, "enrich"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.queueType != tt.expected {
				t.Errorf("expected queue type %q, got %q", tt.expected, tt.queueType)
			}
		})
	}
}

func TestIngestPayload(t *testing.T) {
	payload := IngestPayload{
		SourceKey: "linkedin",
	}

	if payload.SourceKey != "linkedin" {
		t.Errorf("expected source key 'linkedin', got %q", payload.SourceKey)
	}
	if payload.SearchID != nil {
		t.Error("expected nil search ID")
	}
	if payload.SubscriptionID != nil {
		t.Error("expected nil subscription ID")
	}
}

func TestIngestPayloadWithSearchID(t *testing.T) {
	searchID := "search-123"
	payload := IngestPayload{
		SourceKey: "djinni",
		SearchID:  &searchID,
	}

	if payload.SearchID == nil || *payload.SearchID != "search-123" {
		t.Error("expected search ID 'search-123'")
	}
}

func TestIngestPayloadWithSubscriptionID(t *testing.T) {
	subID := "sub-456"
	payload := IngestPayload{
		SourceKey:      "dou",
		SubscriptionID: &subID,
	}

	if payload.SubscriptionID == nil || *payload.SubscriptionID != "sub-456" {
		t.Error("expected subscription ID 'sub-456'")
	}
}

func TestMatchPayload(t *testing.T) {
	payload := MatchPayload{
		JobID: "job-456",
	}

	if payload.JobID != "job-456" {
		t.Errorf("expected job ID 'job-456', got %q", payload.JobID)
	}
}

func TestEnrichPayload(t *testing.T) {
	payload := EnrichPayload{
		JobID: "job-789",
	}

	if payload.JobID != "job-789" {
		t.Errorf("expected job ID 'job-789', got %q", payload.JobID)
	}
}

func TestGeneratePayload(t *testing.T) {
	profileID := "profile-012"
	payload := GeneratePayload{
		JobID:     "job-456",
		Type:      "resume",
		ProfileID: &profileID,
	}

	if payload.JobID != "job-456" {
		t.Errorf("expected job ID 'job-456', got %q", payload.JobID)
	}
	if payload.Type != "resume" {
		t.Errorf("expected type 'resume', got %q", payload.Type)
	}
	if payload.ProfileID == nil || *payload.ProfileID != "profile-012" {
		t.Error("expected profile ID 'profile-012'")
	}
}

func TestGeneratePayloadWithoutProfile(t *testing.T) {
	payload := GeneratePayload{
		JobID: "job-456",
		Type:  "cover_letter",
	}

	if payload.ProfileID != nil {
		t.Error("expected nil profile ID")
	}
}

func TestNewRedisClient(t *testing.T) {
	tests := []struct {
		name     string
		redisURL string
		wantErr  bool
	}{
		{
			name:     "default URL",
			redisURL: "",
			wantErr:  false,
		},
		{
			name:     "custom URL",
			redisURL: "redis://custom-host:6380",
			wantErr:  false,
		},
		{
			name:     "URL with password",
			redisURL: "redis://:secret@localhost:6379",
			wantErr:  false,
		},
		{
			name:     "URL with DB",
			redisURL: "redis://localhost:6379/1",
			wantErr:  false,
		},
		{
			name:     "invalid URL",
			redisURL: "://invalid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRedisClient(tt.redisURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRedisClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func redisClientOptions(t *testing.T, redisURL string) *redis.Options {
	t.Helper()
	client, err := NewRedisClient(redisURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc, ok := client.(*redis.Client)
	if !ok {
		t.Fatalf("expected *redis.Client, got %T", client)
	}
	return rc.Options()
}

func TestNewRedisClientDefault(t *testing.T) {
	opt := redisClientOptions(t, "")

	if opt.Addr != "localhost:6379" {
		t.Errorf("expected addr 'localhost:6379', got %q", opt.Addr)
	}
	if opt.Password != "" {
		t.Errorf("expected empty password, got %q", opt.Password)
	}
	if opt.DB != 0 {
		t.Errorf("expected DB 0, got %d", opt.DB)
	}
}

func TestNewRedisClientCustom(t *testing.T) {
	opt := redisClientOptions(t, "redis://custom-host:6380")

	if opt.Addr != "custom-host:6380" {
		t.Errorf("expected addr 'custom-host:6380', got %q", opt.Addr)
	}
}

func TestNewRedisClientWithPassword(t *testing.T) {
	opt := redisClientOptions(t, "redis://:secret@localhost:6379")

	if opt.Password != "secret" {
		t.Errorf("expected password 'secret', got %q", opt.Password)
	}
}

func TestNewRedisClientWithDB(t *testing.T) {
	opt := redisClientOptions(t, "redis://localhost:6379/5")

	if opt.DB != 5 {
		t.Errorf("expected DB 5, got %d", opt.DB)
	}
}

func TestQueueConstants(t *testing.T) {
	if TypeIngest != "ingest" {
		t.Errorf("expected TypeIngest 'ingest', got %q", TypeIngest)
	}
	if TypeMatch != "match" {
		t.Errorf("expected TypeMatch 'match', got %q", TypeMatch)
	}
	if TypeGenerate != "generate" {
		t.Errorf("expected TypeGenerate 'generate', got %q", TypeGenerate)
	}
	if TypeEnrich != "enrich" {
		t.Errorf("expected TypeEnrich 'enrich', got %q", TypeEnrich)
	}
}

func TestQueueNames(t *testing.T) {
	if QueueIngest != "ingest" {
		t.Errorf("expected QueueIngest 'ingest', got %q", QueueIngest)
	}
	if QueueMatch != "match" {
		t.Errorf("expected QueueMatch 'match', got %q", QueueMatch)
	}
	if QueueGenerate != "generate" {
		t.Errorf("expected QueueGenerate 'generate', got %q", QueueGenerate)
	}
	if QueueEnrich != "enrich" {
		t.Errorf("expected QueueEnrich 'enrich', got %q", QueueEnrich)
	}
}

func TestIngestPayloadEmpty(t *testing.T) {
	payload := IngestPayload{}

	if payload.SourceKey != "" {
		t.Errorf("expected empty source key, got %q", payload.SourceKey)
	}
	if payload.SearchID != nil {
		t.Error("expected nil search ID")
	}
	if payload.SubscriptionID != nil {
		t.Error("expected nil subscription ID")
	}
}

func TestMatchPayloadEmpty(t *testing.T) {
	payload := MatchPayload{}

	if payload.JobID != "" {
		t.Errorf("expected empty job ID, got %q", payload.JobID)
	}
}

func TestEnrichPayloadEmpty(t *testing.T) {
	payload := EnrichPayload{}

	if payload.JobID != "" {
		t.Errorf("expected empty job ID, got %q", payload.JobID)
	}
}

func TestGeneratePayloadEmpty(t *testing.T) {
	payload := GeneratePayload{}

	if payload.JobID != "" {
		t.Errorf("expected empty job ID, got %q", payload.JobID)
	}
	if payload.Type != "" {
		t.Errorf("expected empty type, got %q", payload.Type)
	}
	if payload.ProfileID != nil {
		t.Error("expected nil profile ID")
	}
}
