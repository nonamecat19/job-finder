package dto

type CompanyIntelDto struct {
	CompanyName     string   `json:"companyName"`
	Website         *string  `json:"website"`
	Funding         *string  `json:"funding"`
	Layoffs         *string  `json:"layoffs"`
	GlassdoorRating *float64 `json:"glassdoorRating"`
	Headcount       *string  `json:"headcount"`
	TechStack       *string  `json:"techStack"`
	FetchedAt       string   `json:"fetchedAt"`
	Error           *string  `json:"error,omitempty"`
}

type JobContactDto struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Title       *string `json:"title"`
	LinkedInUrl *string `json:"linkedInUrl"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Source      string  `json:"source"`
	Confidence  float64 `json:"confidence"`
	FetchedAt   string  `json:"fetchedAt"`
}

type GroundingTraceDto struct {
	Claim       string `json:"claim"`
	SignalKind  string `json:"signalKind"`
	SignalValue string `json:"signalValue"`
}

type OutreachDraftDto struct {
	JobID           string              `json:"jobId"`
	ContactID       *string             `json:"contactId"`
	ContactName     *string             `json:"contactName"`
	Tone            string              `json:"tone"`
	Text            string              `json:"text"`
	GroundingTraces []GroundingTraceDto `json:"groundingTraces"`
	GeneratedAt     string              `json:"generatedAt"`
}

type OutreachToneOptionDto struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}
