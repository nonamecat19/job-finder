package domain

import "time"

type Job struct {
	ID              string
	DedupeKey       string
	SourceKey       string
	SubscriptionID  *string
	ExternalID      *string
	Title           string
	Company         string
	Location        *string
	Remote          bool
	SalaryRaw       *string
	URL             string
	Description     string
	DescriptionHTML *string
	PostedAt        *time.Time
	IngestedAt      time.Time
	Status          JobStatus
	Raw             []byte

	SalaryMin        *int
	SalaryMax        *int
	SalaryCurrency   *string
	SalaryConfidence *float64
	SalarySource     *string

	// EmbeddingHash is the hash of the exact text last embedded (019-ai-job-
	// throughput); nil means no embedding has been computed yet.
	ExperienceLevel       *string
	ExperienceMinYears    *int
	EnglishLevel          *string
	SalaryEstimateRaw     *string
	SalaryEstimateMin     *int
	SalaryEstimateMax     *int
	SalaryEstimateCurrency *string

	// EmbeddingHash is the hash of the exact text last embedded (019-ai-job-
	// throughput); nil means no embedding has been computed yet.
	EmbeddingHash *string
	// Embedding is the job's stored vector, reused when EmbeddingHash matches
	// the current content so matching can skip re-embedding.
	Embedding []float32
}

type JobStatus string

const (
	JobStatusNew         JobStatus = "new"
	JobStatusShortlisted JobStatus = "shortlisted"
	JobStatusHidden      JobStatus = "hidden"
)

type MatchResult struct {
	ID            string
	JobID         string
	Similarity    float64
	Score         *int
	MatchedSkills []string
	MissingSkills []string
	Summary       *string
	RedFlags      []string
	Model         string
	CreatedAt     time.Time
}
