package dto

type SourceKind string

const (
	SourceKindAPI     SourceKind = "api"
	SourceKindScrape  SourceKind = "scrape"
	SourceKindSidecar SourceKind = "sidecar"
	// SourceKindManual backs hand-entered vacancies on hosts no adapter reads.
	// It is never crawled — its adapter's Search fails permanently (041 D4).
	SourceKindManual SourceKind = "manual"
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
