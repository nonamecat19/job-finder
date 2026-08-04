package dto

type AiFeatureSettingDto struct {
	Feature   string `json:"feature"`
	Enabled   bool   `json:"enabled"`
	Threshold int    `json:"threshold"`
}

// ResumeShapeConfigDto is the wire form of the resume generation shape
// settings — a field-for-field mirror of generation/domain.ShapeConfig. It is
// both the request and the response body of /v1/settings/resume-shape; PUT
// replaces the whole config, so every field must be present.
//
// 0 means "unlimited / no limit" for skillsMaxGroups, projectsMin, projectsMax,
// projectBulletsMax, certificationsMin and certificationsMax.
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
