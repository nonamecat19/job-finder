// Package dto defines the wire-format structs served by the HTTP API.
// These are the Go source of truth for tygo-generated dashboard TypeScript
// types; field names/JSON tags must match packages/shared/src/index.ts
// exactly (field-for-field) so the existing React dashboard needs zero changes.
package dto

// Enums (mirrored as string types + constant lists, matching the TS `as const` arrays).

type SourceKind string

const (
	SourceKindAPI     SourceKind = "api"
	SourceKindScrape  SourceKind = "scrape"
	SourceKindSidecar SourceKind = "sidecar"
)

type ApplicationStatus string

const (
	StatusFound         ApplicationStatus = "found"
	StatusShortlisted   ApplicationStatus = "shortlisted"
	StatusDocsGenerated ApplicationStatus = "docs_generated"
	StatusApplied       ApplicationStatus = "applied"
	StatusInterview     ApplicationStatus = "interview"
	StatusOffer         ApplicationStatus = "offer"
	StatusRejected      ApplicationStatus = "rejected"
)

var ApplicationStatuses = []ApplicationStatus{
	StatusFound, StatusShortlisted, StatusDocsGenerated, StatusApplied, StatusInterview, StatusOffer, StatusRejected,
}

func IsValidApplicationStatus(s string) bool {
	for _, v := range ApplicationStatuses {
		if string(v) == s {
			return true
		}
	}
	return false
}

type DocumentType string

const (
	DocumentTypeResume      DocumentType = "resume"
	DocumentTypeCoverLetter DocumentType = "cover_letter"
)

// ---------------------------------------------------------------------------
// Job ingestion
// ---------------------------------------------------------------------------

// NormalizedJob is the canonical job shape every source adapter must produce.
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
	PostedAt    *string `json:"postedAt,omitempty"` // ISO date
	Raw         any     `json:"raw"`
}

// SearchQuery is the query shape a saved search stores and adapters receive.
type SearchQuery struct {
	Keywords  string   `json:"keywords"`
	Location  *string  `json:"location,omitempty"`
	Remote    *bool    `json:"remote,omitempty"`
	SalaryMin *float64 `json:"salaryMin,omitempty"`
	Country   *string  `json:"country,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	Site      *string  `json:"site,omitempty"` // 'linkedin' | 'indeed' | 'glassdoor'
	// SubscriptionURL, when set, tells an adapter to scrape this saved-filter
	// URL (e.g. a djinni subs page) instead of running a keyword search.
	SubscriptionURL string `json:"subscriptionUrl,omitempty"`
}

// ---------------------------------------------------------------------------
// JSON Resume (subset of the standard schema we use)
// ---------------------------------------------------------------------------

type ResumeLocation struct {
	City        *string `json:"city,omitempty"`
	CountryCode *string `json:"countryCode,omitempty"`
	Region      *string `json:"region,omitempty"`
}

type ResumeProfile struct {
	Network  *string `json:"network,omitempty"`
	Username *string `json:"username,omitempty"`
	URL      *string `json:"url,omitempty"`
}

type ResumeBasics struct {
	Name     *string         `json:"name,omitempty"`
	Label    *string         `json:"label,omitempty"`
	Email    *string         `json:"email,omitempty"`
	Phone    *string         `json:"phone,omitempty"`
	URL      *string         `json:"url,omitempty"`
	Summary  *string         `json:"summary,omitempty"`
	Location *ResumeLocation `json:"location,omitempty"`
	Profiles []ResumeProfile `json:"profiles,omitempty"`
}

type ResumeWork struct {
	Name       string   `json:"name"`
	Position   *string  `json:"position,omitempty"`
	URL        *string  `json:"url,omitempty"`
	StartDate  *string  `json:"startDate,omitempty"`
	EndDate    *string  `json:"endDate,omitempty"`
	Summary    *string  `json:"summary,omitempty"`
	Highlights []string `json:"highlights,omitempty"`
}

type ResumeEducation struct {
	Institution string  `json:"institution"`
	Area        *string `json:"area,omitempty"`
	StudyType   *string `json:"studyType,omitempty"`
	StartDate   *string `json:"startDate,omitempty"`
	EndDate     *string `json:"endDate,omitempty"`
}

type ResumeSkill struct {
	Name     string   `json:"name"`
	Level    *string  `json:"level,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

type ResumeProject struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	URL         *string  `json:"url,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Highlights  []string `json:"highlights,omitempty"`
}

type ResumeLanguage struct {
	Language *string `json:"language,omitempty"`
	Fluency  *string `json:"fluency,omitempty"`
}

type ResumeCertificate struct {
	Name   *string `json:"name,omitempty"`
	Issuer *string `json:"issuer,omitempty"`
	Date   *string `json:"date,omitempty"`
}

type JsonResume struct {
	Basics       *ResumeBasics       `json:"basics,omitempty"`
	Work         []ResumeWork        `json:"work,omitempty"`
	Education    []ResumeEducation   `json:"education,omitempty"`
	Skills       []ResumeSkill       `json:"skills,omitempty"`
	Projects     []ResumeProject     `json:"projects,omitempty"`
	Languages    []ResumeLanguage    `json:"languages,omitempty"`
	Certificates []ResumeCertificate `json:"certificates,omitempty"`
}

// ---------------------------------------------------------------------------
// API DTOs
// ---------------------------------------------------------------------------

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

type JobDto struct {
	ID              string                 `json:"id"`
	DedupeKey       string                 `json:"dedupeKey"`
	SourceKey       string                 `json:"sourceKey"`
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
	Status          string                 `json:"status"` // ApplicationStatus | 'hidden'
	MatchResult     *MatchResultDto        `json:"matchResult,omitempty"`
	Documents       []GeneratedDocumentDto `json:"documents,omitempty"`
	Application     *ApplicationDto        `json:"application,omitempty"`
}

type JobListResponse struct {
	Items    []JobDto `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

type GeneratedDocumentDto struct {
	ID        string  `json:"id"`
	JobID     string  `json:"jobId"`
	Type      string  `json:"type"`
	Version   int     `json:"version"`
	Content   any     `json:"content"`
	PdfPath   *string `json:"pdfPath"`
	Model     string  `json:"model"`
	CreatedAt string  `json:"createdAt"`
}

type RendercvSummaryExperience struct {
	Company        string `json:"company"`
	HighlightCount int    `json:"highlightCount"`
}

type RendercvSummary struct {
	Name        string                      `json:"name"`
	Headline    string                      `json:"headline"`
	SkillGroups []string                    `json:"skillGroups"`
	Experience  []RendercvSummaryExperience `json:"experience"`
}

type ProfileDto struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	HasConfig      bool             `json:"hasConfig"`
	RendercvConfig *RendercvSummary `json:"rendercvConfig,omitempty"`
	ExtraNotes     *string          `json:"extraNotes"`
	UpdatedAt      string           `json:"updatedAt"`
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

// SubscriptionDto is a URL-based subscription attached to a job source: a
// saved-filter URL on the site itself (e.g. a djinni subs page or a dou
// category listing). Fetching the URL is deferred; this is CRUD + enable only.
type SubscriptionDto struct {
	ID        string  `json:"id"`
	SourceKey string  `json:"sourceKey"`
	Name      *string `json:"name"`
	URL       string  `json:"url"`
	Enabled   bool    `json:"enabled"`
	LastRunAt *string `json:"lastRunAt"`
}

type SourceRunDto struct {
	ID         string  `json:"id"`
	SourceKey  string  `json:"sourceKey"`
	SearchID   *string `json:"searchId"`
	StartedAt  string  `json:"startedAt"`
	FinishedAt *string `json:"finishedAt"`
	OK         *bool   `json:"ok"`
	Found      int     `json:"found"`
	New        int     `json:"new"`
	Error      *string `json:"error"`
}

type ApplicationEvent struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

type ApplicationDto struct {
	ID        string             `json:"id"`
	JobID     string             `json:"jobId"`
	Status    ApplicationStatus  `json:"status"`
	Notes     *string            `json:"notes"`
	AppliedAt *string            `json:"appliedAt"`
	Events    []ApplicationEvent `json:"events"`
	UpdatedAt string             `json:"updatedAt"`
	Job       *JobDto            `json:"job,omitempty"`
}

type StatsDto struct {
	JobsTotal   int64            `json:"jobsTotal"`
	JobsLast24h int64            `json:"jobsLast24h"`
	HighFit     int64            `json:"highFit"`
	Pipeline    map[string]int64 `json:"pipeline"`
	RecentRuns  []SourceRunDto   `json:"recentRuns"`
}

type GenerateRequestDto struct {
	Type      DocumentType `json:"type"`
	ProfileID *string      `json:"profileId,omitempty"`
}

// ActivityRunDto is one row of under-the-hood async task activity (ingest,
// match, generate, enrich), live progress and recent history alike.
type ActivityRunDto struct {
	ID         string         `json:"id"`
	Op         string         `json:"op"`
	State      string         `json:"state"`
	Label      string         `json:"label"`
	Step       *string        `json:"step"`
	JobID      *string        `json:"jobId"`
	SourceKey  *string        `json:"sourceKey"`
	RefID      *string        `json:"refId"`
	Error      *string        `json:"error"`
	Meta       map[string]any `json:"meta"`
	CreatedAt  string         `json:"createdAt"`
	StartedAt  *string        `json:"startedAt"`
	FinishedAt *string        `json:"finishedAt"`
	ElapsedMs  *int64         `json:"elapsedMs"`
}

type ActivityListResponse struct {
	Active []ActivityRunDto `json:"active"`
	Recent []ActivityRunDto `json:"recent"`
}
