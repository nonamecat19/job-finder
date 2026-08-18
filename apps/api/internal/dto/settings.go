package dto

type AiFeatureSettingDto struct {
	Feature   string `json:"feature"`
	Enabled   bool   `json:"enabled"`
	Threshold int    `json:"threshold"`
}

type ResumeShapeConfigDto struct {
	SummaryLines          int  `json:"summaryLines"`
	SummaryEnabled        bool `json:"summaryEnabled"`
	SkillsEnabled         bool `json:"skillsEnabled"`
	SkillsMaxGroups       int  `json:"skillsMaxGroups"`
	ExperienceEnabled     bool `json:"experienceEnabled"`
	ExperienceBulletsMin  int  `json:"experienceBulletsMin"`
	ExperienceBulletsMax  int  `json:"experienceBulletsMax"`
	TargetPages           int  `json:"targetPages"`
	ProjectsEnabled       bool `json:"projectsEnabled"`
	ProjectsMin           int  `json:"projectsMin"`
	ProjectsMax           int  `json:"projectsMax"`
	ProjectBulletsMax     int  `json:"projectBulletsMax"`
	CertificationsEnabled bool `json:"certificationsEnabled"`
	CertificationsMin     int  `json:"certificationsMin"`
	CertificationsMax     int  `json:"certificationsMax"`
	EducationEnabled      bool `json:"educationEnabled"`
	FontSize              int  `json:"fontSize"`
}

type SummaryModelOptionDto struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Cost        string `json:"cost"`
	Current     bool   `json:"current"`
}

type SummaryModelSettingDto struct {
	Options  []SummaryModelOptionDto `json:"options"`
	OptionID string                  `json:"optionId"`
}

type UpdateSummaryModelRequestDto struct {
	OptionID string `json:"optionId"`
}
