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

// ExtProfileDto is the profile shape exposed to the browser extension via
// GET /api/v1/ext/profile (spec 014-autofill-extension section 3). It is a
// deliberately narrower, flatter projection of ProfileDto/RendercvMaster —
// only the fields an application-form autofill needs, nothing else from the
// account (no rendercv theme/design, no other profiles, no internal ids
// beyond the leaf entries below).
type ExtProfileDto struct {
	FullName    string         `json:"fullName"`
	Email       string         `json:"email"`
	Phone       string         `json:"phone"`
	Location    string         `json:"location"`
	Headline    string         `json:"headline"`
	Skills      []string       `json:"skills"`
	WorkHistory []ExtWorkEntry `json:"workHistory"`
	Education   []ExtEducation `json:"education"`
	Links       []ExtLink      `json:"links"`
}

type ExtWorkEntry struct {
	Employer    string  `json:"employer"`
	Role        string  `json:"role"`
	StartDate   string  `json:"startDate"`
	EndDate     *string `json:"endDate"`
	Current     bool    `json:"current"`
	Description string  `json:"description"`
}

type ExtEducation struct {
	Institution string `json:"institution"`
	Degree      string `json:"degree"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
}

type ExtLink struct {
	URL   string `json:"url"`
	Label string `json:"label"`
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
// category listing). Cron is the schedule the ingestion scheduler scrapes it
// on, same expression format as SavedSearchDto.Cron.
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

// FitGapEvidenceDto is one adjacent profile entry offered as evidence for a
// missing must-have (009 fit-gap coach), with a grounded rephrase suggestion
// truthfully reframing that existing bullet toward the missing term.
type FitGapEvidenceDto struct {
	SourceEntry  string `json:"sourceEntry"`
	SourceBullet string `json:"sourceBullet"`
	Proximity    string `json:"proximity"` // "close" | "moderate" | "distant"
	Rephrase     string `json:"rephrase"`
}

// FitGapItemDto is one missing must-have with up to 3 adjacent evidence
// items drawn from the user's profile. NoAdjacentEvidence is the honest
// empty result: nothing in the profile is close enough to cite.
type FitGapItemDto struct {
	Term               string              `json:"term"`
	Polarity           string              `json:"polarity"` // always "required"
	AdjacentEvidence   []FitGapEvidenceDto `json:"adjacentEvidence"`
	NoAdjacentEvidence bool                `json:"noAdjacentEvidence"`
}

// FitGapAssessmentDto is the fit-gap coach output (009), served by
// POST /api/jobs/{id}/coach/assess and GET /api/jobs/{id}/coach/assessment:
// "you fail N of M must-haves", plus per-gap adjacent evidence.
type FitGapAssessmentDto struct {
	JobID           string          `json:"jobId"`
	TotalMustHaves  int             `json:"totalMustHaves"`
	FailedMustHaves int             `json:"failedMustHaves"`
	CoveragePct     float64         `json:"coveragePct"`
	Gaps            []FitGapItemDto `json:"gaps"`
}

// ---------------------------------------------------------------------------
// Interview prep pack
// ---------------------------------------------------------------------------

// StoryMappingDto is one STAR story matched to an interview question, with
// its relevance score and the skills it shares with the question.
type StoryMappingDto struct {
	StoryID        string   `json:"storyId"`
	StoryTitle     string   `json:"storyTitle"`
	RelevanceScore float64  `json:"relevanceScore"`
	MatchedSkills  []string `json:"matchedSkills"`
	Excerpt        string   `json:"excerpt"`
}

// InterviewQuestionDto is one derived interview question with its best
// matching STAR stories (empty when nothing in the profile covers it).
type InterviewQuestionDto struct {
	ID            string            `json:"id"`
	Text          string            `json:"text"`
	Category      string            `json:"category"`
	Source        string            `json:"source"`
	SourceExcerpt string            `json:"sourceExcerpt"`
	MappedStories []StoryMappingDto `json:"mappedStories"`
}

// CompanyNewsItemDto is one company-intel signal reshaped as a briefing item
// for the interview prep pack (spec 013 reuses 004's Company/CompanySignal
// data rather than a dedicated news source).
type CompanyNewsItemDto struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetchedAt"`
}

// KeywordGapSummaryDto reshapes the 008 keyword diff into the prep pack's
// gap-awareness section: just the missing term strings, coverage, and a few
// templated tips (there is no dedicated "tips" source, so these are derived
// from the missing terms themselves).
type KeywordGapSummaryDto struct {
	MissingRequired  []string `json:"missingRequired"`
	MissingPreferred []string `json:"missingPreferred"`
	CoveragePct      float64  `json:"coveragePct"`
	GapAwarenessTips []string `json:"gapAwarenessTips"`
}

// InterviewPrepMetadataDto carries the coverage counters and staleness flag
// the panel renders in its header.
type InterviewPrepMetadataDto struct {
	TotalQuestions     int  `json:"totalQuestions"`
	CoveredQuestions   int  `json:"coveredQuestions"`
	UncoveredQuestions int  `json:"uncoveredQuestions"`
	StaleNews          bool `json:"staleNews"`
}

// InterviewPrepPackDto is the full interview prep pack served by
// GET /api/jobs/{id}/interview-prep (spec 013).
type InterviewPrepPackDto struct {
	JobID       string                   `json:"jobId"`
	GeneratedAt string                   `json:"generatedAt"`
	Questions   []InterviewQuestionDto   `json:"questions"`
	CompanyNews []CompanyNewsItemDto     `json:"companyNews"`
	KeywordGap  KeywordGapSummaryDto     `json:"keywordGap"`
	Metadata    InterviewPrepMetadataDto `json:"metadata"`
}

// JobContactDto is one resolved recruiter/hiring-manager candidate served
// by GET /api/jobs/{id}/contacts and POST /api/jobs/{id}/contacts/refresh
// (spec 007). Nil fields mean that channel was never resolved — never
// fabricated (FR-006, FR-008). Email/phone/linkedInUrl are sensitive
// (FR-018): the API surfaces them only on this endpoint, and callers must
// not log them in full.
type JobContactDto struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Title       *string `json:"title"`
	LinkedInUrl *string `json:"linkedInUrl"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	// Source is one of "posting" / "company-page" / "linkedin".
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	FetchedAt  string  `json:"fetchedAt"`
}

// ReferralContactDto is one hop in a warm-path chain — a contact imported
// from CSV or discovered via GitHub cross-reference.
type ReferralContactDto struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Email          *string `json:"email,omitempty"`
	Company        *string `json:"company,omitempty"`
	Role           *string `json:"role,omitempty"`
	LinkedInUrl    *string `json:"linkedInUrl,omitempty"`
	GitHubUsername *string `json:"gitHubUsername,omitempty"`
}

// ReferralPathDto is one ranked warm path from the user to a contact at the
// job's company, served by GET /api/jobs/{id}/referral-paths.
type ReferralPathDto struct {
	Path   []ReferralContactDto `json:"path"`
	Score  float64              `json:"score"`
	Length int                  `json:"length"`
}

// ContactImportResultDto reports the outcome of a contacts CSV import,
// served by POST /api/contacts/import.
type ContactImportResultDto struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Total    int `json:"total"`
}

// GithubSyncResultDto reports the outcome of cross-referencing one contact's
// GitHub followers/following against the existing contact book, served by
// POST /api/contacts/{id}/github-sync.
type GithubSyncResultDto struct {
	Contact          ReferralContactDto `json:"contact"`
	FollowersScanned int                `json:"followersScanned"`
	FollowingScanned int                `json:"followingScanned"`
	ConnectionsMade  int                `json:"connectionsMade"`
}

// ---------------------------------------------------------------------------
// Post-apply outreach draft generator (012)
// ---------------------------------------------------------------------------

// GroundingTraceDto is one specific claim in an OutreachDraftDto mapped to
// the company-intel signal that backs it (spec 012 FR-014) — the wire shape
// of the Story 3 "verify before you send it" view.
type GroundingTraceDto struct {
	Claim       string `json:"claim"`
	SignalKind  string `json:"signalKind"`
	SignalValue string `json:"signalValue"`
}

// OutreachDraftDto is a generated, never-sendable outreach message served
// by POST /api/jobs/{id}/outreach/generate (spec 012). ContactId/ContactName
// are nil when no resolved contact exists for the job (a neutral salutation
// was used instead — FR-007). GroundingTraces is always a non-nil (possibly
// empty) slice: empty means the draft made no specific claim at all, which
// is the honest fallback when no company-intel signal exists (FR-012).
type OutreachDraftDto struct {
	JobID           string              `json:"jobId"`
	ContactID       *string             `json:"contactId"`
	ContactName     *string             `json:"contactName"`
	Tone            string              `json:"tone"`
	Text            string              `json:"text"`
	GroundingTraces []GroundingTraceDto `json:"groundingTraces"`
	GeneratedAt     string              `json:"generatedAt"`
}

// OutreachToneOptionDto is one offered tone option, served by
// GET /api/jobs/{id}/outreach/tones (FR-010, FR-011).
type OutreachToneOptionDto struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// ---------------------------------------------------------------------------
// Cerebras free-tier model toggle (001-cerebras-model-toggle)
// ---------------------------------------------------------------------------

// LlmTaskSettingDto is one chat task's assigned provider/model, served by
// GET/PUT /v1/settings/llm. TaskKey is one of llmsettings.TaskKeys ("match",
// "generation", "rephrase", "ghost", "default"); Provider is "ollama",
// "cerebras" or "openrouter". Model is "" when the provider's own default
// model applies.
type LlmTaskSettingDto struct {
	TaskKey  string `json:"taskKey"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// LlmSettingsResponseDto is the GET/PUT /v1/settings/llm response.
// CredentialConfigured / OpenRouterCredentialConfigured reflect whether
// CEREBRAS_API_KEY / OPENROUTER_API_KEY were set at process start — never the
// keys themselves, which never leave the server (FR-011, FR-013).
type LlmSettingsResponseDto struct {
	CredentialConfigured           bool                `json:"credentialConfigured"`
	OpenRouterCredentialConfigured bool                `json:"openRouterCredentialConfigured"`
	Tasks                          []LlmTaskSettingDto `json:"tasks"`
}

// UpdateLlmSettingsRequestDto is the PUT /v1/settings/llm request body. Only
// the included tasks are changed; omitted tasks keep their current setting.
type UpdateLlmSettingsRequestDto struct {
	Tasks []LlmTaskSettingDto `json:"tasks"`
}

// AiFeatureSettingDto is one row of the GET /v1/settings/ai-features response
// / PUT /v1/settings/ai-features/{feature} body: whether an AI feature
// (resume generation, cover letter generation, salary inference) is
// auto-enqueued when a job's match score reaches Threshold (0-100). Below
// the threshold, or when disabled, the feature only runs on-demand. Match
// scoring itself has no entry — it always runs unconditionally.
type AiFeatureSettingDto struct {
	Feature   string `json:"feature"`
	Enabled   bool   `json:"enabled"`
	Threshold int    `json:"threshold"`
}

// CerebrasModelDto is one curated Cerebras free-tier model offered in the
// Settings model selector, served by GET /v1/settings/llm/models.
type CerebrasModelDto struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDefault bool   `json:"isDefault"`
}

// OpenRouterModelDto is one curated OpenRouter model offered in the Settings
// model selector, served by GET /v1/settings/llm/models.
type OpenRouterModelDto struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDefault bool   `json:"isDefault"`
}

// LlmModelsResponseDto is the GET /v1/settings/llm/models response: the
// curated model list per remote provider.
type LlmModelsResponseDto struct {
	Cerebras   []CerebrasModelDto   `json:"cerebras"`
	OpenRouter []OpenRouterModelDto `json:"openrouter"`
}
