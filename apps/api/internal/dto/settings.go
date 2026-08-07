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
