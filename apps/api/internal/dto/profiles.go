package dto

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
