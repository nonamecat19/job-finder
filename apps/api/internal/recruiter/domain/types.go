package domain

type ResolvedContact struct {
	Name        string
	Title       *string
	LinkedInURL *string
	Email       *string
	Phone       *string
	Source      string
	Confidence  float64
}

const (
	SourcePosting     = "posting"
	SourceCompanyPage = "company-page"
	SourceLinkedIn    = "linkedin"
)
