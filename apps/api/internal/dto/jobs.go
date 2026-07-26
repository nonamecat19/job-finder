package dto

import "time"

type NormalizedJob struct {
	SourceKey   string  `json:"sourceKey"`
	ExternalID  *string `json:"externalId,omitempty"`
	Title       string  `json:"title"`
	Company     string  `json:"company"`
	Location    *string `json:"location,omitempty"`
	Remote      bool    `json:"remote"`
	SalaryRaw   *string `json:"salaryRaw,omitempty"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	PostedAt    *string `json:"postedAt,omitempty"`
	Raw         any     `json:"raw"`
}

type SearchQuery struct {
	Keywords  string   `json:"keywords"`
	Location  *string  `json:"location,omitempty"`
	Remote    *bool    `json:"remote,omitempty"`
	SalaryMin *float64 `json:"salaryMin,omitempty"`
	Country   *string  `json:"country,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	SubscriptionURL string `json:"subscriptionUrl,omitempty"`
}

type JobDto struct {
	ID              string                 `json:"id"`
	DedupeKey       string                 `json:"dedupeKey"`
	SourceKey       string                 `json:"sourceKey"`
	SubscriptionID  *string                `json:"subscriptionId"`
	Title           string                 `json:"title"`
	Company         string                 `json:"company"`
	Location        *string                `json:"location"`
	Remote          bool                   `json:"remote"`
	SalaryRaw       *string                `json:"salaryRaw"`
	URL             string                 `json:"url"`
	Description     string                 `json:"description"`
	DescriptionHtml *string                `json:"descriptionHtml,omitempty"`
	PostedAt        *string                `json:"postedAt"`
	IngestedAt      string                 `json:"ingestedAt"`
	Status          string                 `json:"status"`
	MatchResult     *MatchResultDto        `json:"matchResult,omitempty"`
	Documents       []GeneratedDocumentDto `json:"documents,omitempty"`
	Application     *ApplicationDto        `json:"application,omitempty"`
	SalaryMin        *int     `json:"salaryMin"`
	SalaryMax        *int     `json:"salaryMax"`
	SalaryCurrency   *string  `json:"salaryCurrency"`
	SalaryConfidence *float64 `json:"salaryConfidence"`
	SalarySource     *string  `json:"salarySource"`
	SalaryBelowFloor bool     `json:"salaryBelowFloor"`
	GhostSignal *JobSignalDto `json:"ghostSignal,omitempty"`
}

type JobListResponse struct {
	Items    []JobDto `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

type MatchResultDto struct {
	ID            string    `json:"id"`
	JobID         string    `json:"jobId"`
	Similarity    float64   `json:"similarity"`
	Score         *int      `json:"score"`
	MatchedSkills *[]string `json:"matchedSkills"`
	MissingSkills *[]string `json:"missingSkills"`
	Summary       *string   `json:"summary"`
	RedFlags      *[]string `json:"redFlags"`
	Model         string    `json:"model"`
	CreatedAt     string    `json:"createdAt"`
}

type JobSourceDto struct {
	ID      string         `json:"id"`
	Key     string         `json:"key"`
	Kind    SourceKind     `json:"kind"`
	Enabled bool           `json:"enabled"`
	Healthy bool           `json:"healthy"`
	Config  map[string]any `json:"config"`
}

type SavedSearchDto struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Query     SearchQuery `json:"query"`
	Cron      string      `json:"cron"`
	Enabled   bool        `json:"enabled"`
	LastRunAt *string     `json:"lastRunAt"`
}

type SubscriptionDto struct {
	ID        string  `json:"id"`
	SourceKey string  `json:"sourceKey"`
	Name      *string `json:"name"`
	URL       string  `json:"url"`
	Enabled   bool    `json:"enabled"`
	Cron      string  `json:"cron"`
	LastRunAt *string `json:"lastRunAt"`
}

type SourceRunDto struct {
	ID           string  `json:"id"`
	SourceKey    string  `json:"sourceKey"`
	SearchID     *string `json:"searchId"`
	StartedAt    string  `json:"startedAt"`
	FinishedAt   *string `json:"finishedAt"`
	OK           *bool   `json:"ok"`
	Found        int     `json:"found"`
	New          int     `json:"new"`
	Error        *string `json:"error"`
	Verdict      *string `json:"verdict"`
	BlockedCount int     `json:"blockedCount"`
	BlockReason  *string `json:"blockReason"`
}

type RunVerdictDto struct {
	Verdict      string  `json:"verdict"`
	BlockedCount int     `json:"blockedCount"`
	BlockReason  *string `json:"blockReason,omitempty"`
}

type HostRetrievalStatusDto struct {
	Host              string     `json:"host"`
	IdentityVersion   string     `json:"identityVersion"`
	CurrentRung       string     `json:"currentRung"`
	LastBlockAt       *time.Time `json:"lastBlockAt,omitempty"`
	LastBlockReason   *string    `json:"lastBlockReason,omitempty"`
	CoolingOffUntil   *time.Time `json:"coolingOffUntil,omitempty"`
	BudgetUsed        int        `json:"budgetUsed"`
	BudgetLimit       int        `json:"budgetLimit"`
	BudgetResetsAt    time.Time  `json:"budgetResetsAt"`
	CrawlDelaySeconds *int       `json:"crawlDelaySeconds,omitempty"`
}

type EmployerBoardDto struct {
	ID                 string  `json:"id"`
	Vendor             string  `json:"vendor"`
	EmployerIdentifier string  `json:"employerIdentifier"`
	DisplayName        string  `json:"displayName"`
	AddedVia           string  `json:"addedVia"`
	Enabled            bool    `json:"enabled"`
	LastSuccessAt      *string `json:"lastSuccessAt"`
	LastPostingCount   int     `json:"lastPostingCount"`
	Stale              bool    `json:"stale"`
}

type BoardCandidateDto struct {
	ID                 string  `json:"id"`
	Vendor             string  `json:"vendor"`
	EmployerIdentifier string  `json:"employerIdentifier"`
	DisplayName        string  `json:"displayName"`
	InferredFromJobID  *string `json:"inferredFromJobId"`
	State              string  `json:"state"`
}

type GenerateRequestDto struct {
	Type      DocumentType `json:"type"`
	ProfileID *string      `json:"profileId,omitempty"`
}
