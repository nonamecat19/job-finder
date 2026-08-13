package dto

type AiFeatureSettingDto struct {
	Feature   string `json:"feature"`
	Enabled   bool   `json:"enabled"`
	Threshold int    `json:"threshold"`
}

type ResumeShapeConfigDto struct {
	SummaryLines          int  `json:"summaryLines"`
	SkillsEnabled         bool `json:"skillsEnabled"`
	SkillsMaxGroups       int  `json:"skillsMaxGroups"`
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
	FontSize              int  `json:"fontSize"`
}

// SummaryModelOptionDto is one entry on the 034 summary-model menu. Cost is a
// relative indicator, not a price: a figure here would be wrong within a month
// and could not be reproduced by the reader. Real per-run cost lives in the 038
// comparison artifact.
type SummaryModelOptionDto struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Cost        string `json:"cost"`
	Current     bool   `json:"current"`
}

// SummaryModelSettingDto is the whole menu plus which entry is selected, so the
// dashboard renders the selector from one response rather than joining a
// catalogue against a setting.
type SummaryModelSettingDto struct {
	Options  []SummaryModelOptionDto `json:"options"`
	OptionID string                  `json:"optionId"`
}

type UpdateSummaryModelRequestDto struct {
	OptionID string `json:"optionId"`
}
