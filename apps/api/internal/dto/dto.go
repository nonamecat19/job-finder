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

// OutcomeEventType is the append-only outcome event log's enum (spec 010,
// migration 00012_application_outcome.sql). A "response" is any event whose
// type is not OutcomeApplied — silence (only `applied` recorded) is a
// non-response, never an exclusion.
type OutcomeEventType string

const (
	OutcomeApplied  OutcomeEventType = "applied"
	OutcomeViewed   OutcomeEventType = "viewed"
	OutcomeScreen   OutcomeEventType = "screen"
	OutcomeOffer    OutcomeEventType = "offer"
	OutcomeRejected OutcomeEventType = "rejected"
)

var OutcomeEventTypes = []OutcomeEventType{
	OutcomeApplied, OutcomeViewed, OutcomeScreen, OutcomeOffer, OutcomeRejected,
}

func IsValidOutcomeEventType(s string) bool {
	for _, v := range OutcomeEventTypes {
		if string(v) == s {
			return true
		}
	}
	return false
}

// OutcomeEventForStatus maps an application status transition onto the outcome
// event it records. Statuses before submission (found/shortlisted/
// docs_generated) record no outcome event — the log holds real observed
// application outcomes only. OutcomeViewed has no corresponding status: it is
// defined for ATS/employer view signals and simply never fires until a source
// can supply it.
func OutcomeEventForStatus(s ApplicationStatus) (OutcomeEventType, bool) {
	switch s {
	case StatusApplied:
		return OutcomeApplied, true
	case StatusInterview:
		return OutcomeScreen, true
	case StatusOffer:
		return OutcomeOffer, true
	case StatusRejected:
		return OutcomeRejected, true
	default:
		return "", false
	}
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

	// Salary inference (spec 006). All five are nil together when no source
	// could produce a band (FR-009) — SalaryRaw is preserved and displayed
	// alongside regardless (FR-024).
	SalaryMin        *int     `json:"salaryMin"`
	SalaryMax        *int     `json:"salaryMax"`
	SalaryCurrency   *string  `json:"salaryCurrency"`
	SalaryConfidence *float64 `json:"salaryConfidence"`
	SalarySource     *string  `json:"salarySource"`
	// SalaryBelowFloor is computed (not stored) against the configured
	// SALARY_FLOOR_USD; true only when the band's currency is USD and its
	// max lies entirely below the floor (FR-016, FR-020 fail-open for other
	// currencies).
	SalaryBelowFloor bool `json:"salaryBelowFloor"`

	// GhostSignal is the ghost-job detector's (005) result, when one exists.
	// A job with no ghost result renders exactly as it does today — this
	// field is simply absent, never a zero-valued panel (FR-017, SC-008).
	GhostSignal *JobSignalDto `json:"ghostSignal,omitempty"`
}

type JobListResponse struct {
	Items    []JobDto `json:"items"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

type GeneratedDocumentDto struct {
	ID        string  `json:"id"`
	JobID     *string `json:"jobId"`
	Type      string  `json:"type"`
	Version   int     `json:"version"`
	Content   any     `json:"content"`
	PdfPath   *string `json:"pdfPath"`
	Model     string  `json:"model"`
	Company   *string `json:"company,omitempty"`
	Title     *string `json:"title,omitempty"`
	Vacancy   *string `json:"vacancy,omitempty"`
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
	RendercvFull   any              `json:"rendercvFull,omitempty"`
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

// ApplicationOutcomeDto is one row of the append-only outcome event log.
// OccurredAt is when the real-world event happened (may be back-dated);
// RecordedAt is when the row was written and is never back-dated.
type ApplicationOutcomeDto struct {
	ID            string           `json:"id"`
	ApplicationID string           `json:"applicationId"`
	EventType     OutcomeEventType `json:"eventType"`
	OccurredAt    string           `json:"occurredAt"`
	RecordedAt    string           `json:"recordedAt"`
	Note          *string          `json:"note,omitempty"`
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

// KeywordDiffTermDto is one term in a keyword-diff bucket (008-4). The JSON
// shape mirrors the persisted KeywordDiff jsonb (spec 008-1 §3) so the wire
// format and the cache stay identical.
type KeywordDiffTermDto struct {
	Term       string `json:"term"`
	Canonical  string `json:"canonical"`
	Polarity   string `json:"polarity"` // "required" | "preferred"
	Normalized string `json:"normalized"`
	MatchType  string `json:"matchType,omitempty"` // "exact" | "normalized"
}

// KeywordDiffMetadataDto carries the coverage counters the diff panel renders.
type KeywordDiffMetadataDto struct {
	TotalRequired    int     `json:"totalRequired"`
	TotalPreferred   int     `json:"totalPreferred"`
	MatchedRequired  int     `json:"matchedRequired"`
	MatchedPreferred int     `json:"matchedPreferred"`
	CoveragePct      float64 `json:"coveragePct"`
}

// KeywordRephraseSuggestionDto is an advisory, truthful rephrase for a missing
// required term (008-5). Rephrase is null when no honest rephrase is available.
type KeywordRephraseSuggestionDto struct {
	Term         string  `json:"term"`
	Canonical    string  `json:"canonical"`
	Rephrase     *string `json:"rephrase"`
	SourceBullet string  `json:"sourceBullet,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

// KeywordDiffDto is the response served by GET /api/jobs/{id}/keyword-diff:
// the three diff buckets, coverage metadata, and any advisory rephrase
// suggestions for the missing-required terms.
type KeywordDiffDto struct {
	JobID            string                         `json:"jobId"`
	Matched          []KeywordDiffTermDto           `json:"matched"`
	MissingRequired  []KeywordDiffTermDto           `json:"missingRequired"`
	MissingPreferred []KeywordDiffTermDto           `json:"missingPreferred"`
	Metadata         KeywordDiffMetadataDto         `json:"metadata"`
	Suggestions      []KeywordRephraseSuggestionDto `json:"suggestions"`
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

// PostAgeBucketState is the three-state output contract for the post-age
// vs response-rate signal (spec 010 §Cold-Start Honesty).
type PostAgeBucketState string

const (
	PostAgeStateObserved     PostAgeBucketState = "observed"
	PostAgeStatePrior        PostAgeBucketState = "prior"
	PostAgeStateInsufficient PostAgeBucketState = "insufficient"
)

// PostAgeBucketDto is one bucket in the post-age vs response-rate signal.
// Rate is null unless State == PostAgeStateObserved. N is always present
// so the caller can render sample size alongside any rate.
type PostAgeBucketDto struct {
	Bucket    string             `json:"bucket"`
	N         int32              `json:"n"`
	Responses int32              `json:"responses"`
	Rate      *float64           `json:"rate"`
	State     PostAgeBucketState `json:"state"`
}

// PostAgeResponseDto is the full signal response served by
// GET /api/postage-response-rate.
type PostAgeResponseDto struct {
	Buckets      []PostAgeBucketDto `json:"buckets"`
	TotalApps    int32              `json:"totalApps"`
	GlobalState  PostAgeBucketState `json:"globalState"`
	PriorRate    float64            `json:"priorRate"`
	PriorLabel   string             `json:"priorLabel"`
	ThresholdMsg *string            `json:"thresholdMsg,omitempty"`
}

// FreshMatchNotificationDto is one row of the fresh-match notification table,
// served by GET /api/notifications.
type FreshMatchNotificationDto struct {
	ID            string `json:"id"`
	JobId         string `json:"jobId"`
	MatchResultId string `json:"matchResultId"`
	Fresh         bool   `json:"fresh"`
	Seen          bool   `json:"seen"`
	CreatedAt     string `json:"createdAt"`
	// JobTitle and Company are populated by the list endpoint via a JOIN.
	JobTitle   *string `json:"jobTitle,omitempty"`
	Company    *string `json:"company,omitempty"`
	MatchScore *int32  `json:"matchScore,omitempty"`
}

// CompanyIntelDto is the flattened company-intel signal set served by
// GET /api/companies/{jobId}/intel and POST /api/companies/{jobId}/intel/refresh
// (spec 004). Each of the five CompanySignal rows for the job's company is
// flattened into one named field; a nil field means that signal has never
// been captured (or its source failed and no previous value exists yet).
type CompanyIntelDto struct {
	CompanyName     string   `json:"companyName"`
	Website         *string  `json:"website"`
	Funding         *string  `json:"funding"`
	Layoffs         *string  `json:"layoffs"`
	GlassdoorRating *float64 `json:"glassdoorRating"`
	Headcount       *string  `json:"headcount"`
	TechStack       *string  `json:"techStack"`
	FetchedAt       string   `json:"fetchedAt"`
	// Error is set on a Refresh response when every source failed (FR-007):
	// previous values (if any) remain in the other fields, and the
	// dashboard shows a top-level error banner.
	Error *string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Ghost-job detector (005)
// ---------------------------------------------------------------------------

// GhostSignalBreakdownDto is the measured evidence behind one ghost score:
// the four signals (a value or an explicit unknown — never a bare 0), the
// model's confidence, its plain-English explanation, and per-signal
// provenance notes. This is also the exact shape marshaled into
// "JobSignal"."signals" jsonb, so persistence and API response share one
// struct (see ghostjob.Service).
type GhostSignalBreakdownDto struct {
	RepostCount       int               `json:"repostCount"`
	DaysOpen          *int              `json:"daysOpen"`
	CrossBoardCount   *int              `json:"crossBoardCount"`
	AlwaysHiringCount *int              `json:"alwaysHiringCount"`
	Confidence        float64           `json:"confidence"`
	Explanation       string            `json:"explanation"`
	TopSignals        []string          `json:"topSignals,omitempty"`
	Notes             map[string]string `json:"notes"`
}

// JobSignalDto is one row of the generic "JobSignal" table. For this
// feature kind is always "ghost"; the shape is deliberately generic so a
// future signal kind reuses it (spec Key Entities: Job Signal).
type JobSignalDto struct {
	ID        string                  `json:"id"`
	JobID     string                  `json:"jobId"`
	Kind      string                  `json:"kind"`
	Score     int                     `json:"score"`
	Model     string                  `json:"model"`
	CreatedAt string                  `json:"createdAt"`
	Signals   GhostSignalBreakdownDto `json:"signals"`
}
