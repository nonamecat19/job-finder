package dto

type KeywordDiffTermDto struct {
	Term       string `json:"term"`
	Canonical  string `json:"canonical"`
	Polarity   string `json:"polarity"`
	Normalized string `json:"normalized"`
	MatchType  string `json:"matchType,omitempty"`
}

type KeywordDiffMetadataDto struct {
	TotalRequired    int     `json:"totalRequired"`
	TotalPreferred   int     `json:"totalPreferred"`
	MatchedRequired  int     `json:"matchedRequired"`
	MatchedPreferred int     `json:"matchedPreferred"`
	CoveragePct      float64 `json:"coveragePct"`
}

type KeywordRephraseSuggestionDto struct {
	Term         string  `json:"term"`
	Canonical    string  `json:"canonical"`
	Rephrase     *string `json:"rephrase"`
	SourceBullet string  `json:"sourceBullet,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

type KeywordDiffDto struct {
	JobID            string                         `json:"jobId"`
	Matched          []KeywordDiffTermDto           `json:"matched"`
	MissingRequired  []KeywordDiffTermDto           `json:"missingRequired"`
	MissingPreferred []KeywordDiffTermDto           `json:"missingPreferred"`
	Metadata         KeywordDiffMetadataDto         `json:"metadata"`
	Suggestions      []KeywordRephraseSuggestionDto `json:"suggestions"`
}

type FitGapEvidenceDto struct {
	SourceEntry  string `json:"sourceEntry"`
	SourceBullet string `json:"sourceBullet"`
	Proximity    string `json:"proximity"`
	Rephrase     string `json:"rephrase"`
}

type FitGapItemDto struct {
	Term               string              `json:"term"`
	Polarity           string              `json:"polarity"`
	AdjacentEvidence   []FitGapEvidenceDto `json:"adjacentEvidence"`
	NoAdjacentEvidence bool                `json:"noAdjacentEvidence"`
}

type FitGapAssessmentDto struct {
	JobID           string          `json:"jobId"`
	TotalMustHaves  int             `json:"totalMustHaves"`
	FailedMustHaves int             `json:"failedMustHaves"`
	CoveragePct     float64         `json:"coveragePct"`
	Gaps            []FitGapItemDto `json:"gaps"`
}

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

type JobSignalDto struct {
	ID        string                  `json:"id"`
	JobID     string                  `json:"jobId"`
	Kind      string                  `json:"kind"`
	Score     int                     `json:"score"`
	Model     string                  `json:"model"`
	CreatedAt string                  `json:"createdAt"`
	Signals   GhostSignalBreakdownDto `json:"signals"`
}

type StoryMappingDto struct {
	StoryID        string   `json:"storyId"`
	StoryTitle     string   `json:"storyTitle"`
	RelevanceScore float64  `json:"relevanceScore"`
	MatchedSkills  []string `json:"matchedSkills"`
	Excerpt        string   `json:"excerpt"`
}

type InterviewQuestionDto struct {
	ID            string            `json:"id"`
	Text          string            `json:"text"`
	Category      string            `json:"category"`
	Source        string            `json:"source"`
	SourceExcerpt string            `json:"sourceExcerpt"`
	MappedStories []StoryMappingDto `json:"mappedStories"`
}

type CompanyNewsItemDto struct {
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	Source    string `json:"source"`
	FetchedAt string `json:"fetchedAt"`
}

type KeywordGapSummaryDto struct {
	MissingRequired  []string `json:"missingRequired"`
	MissingPreferred []string `json:"missingPreferred"`
	CoveragePct      float64  `json:"coveragePct"`
	GapAwarenessTips []string `json:"gapAwarenessTips"`
}

type InterviewPrepMetadataDto struct {
	TotalQuestions     int  `json:"totalQuestions"`
	CoveredQuestions   int  `json:"coveredQuestions"`
	UncoveredQuestions int  `json:"uncoveredQuestions"`
	StaleNews          bool `json:"staleNews"`
}

type InterviewPrepPackDto struct {
	JobID       string                   `json:"jobId"`
	GeneratedAt string                   `json:"generatedAt"`
	Questions   []InterviewQuestionDto   `json:"questions"`
	CompanyNews []CompanyNewsItemDto     `json:"companyNews"`
	KeywordGap  KeywordGapSummaryDto     `json:"keywordGap"`
	Metadata    InterviewPrepMetadataDto `json:"metadata"`
}
