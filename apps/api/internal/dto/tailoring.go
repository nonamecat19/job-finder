package dto

type EditProposalDto struct {
	ID            string          `json:"id"`
	DraftID       string          `json:"draftId"`
	FieldType     string          `json:"fieldType"`
	FieldKey      string          `json:"fieldKey"`
	BeforeValue   string          `json:"beforeValue"`
	AfterValue    string          `json:"afterValue"`
	Traceability  TraceabilityDto `json:"traceability"`
	Status        string          `json:"status"`
	DroppedReason *string         `json:"droppedReason,omitempty"`
	AcceptedAt    *string         `json:"acceptedAt,omitempty"`
	RejectedAt    *string         `json:"rejectedAt,omitempty"`
}

type TraceabilityDto struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type TailoredDraftDto struct {
	ID               string             `json:"id"`
	ProfileID        string             `json:"profileId"`
	JobID            *string            `json:"jobId,omitempty"`
	VacancyCompany   *string            `json:"vacancyCompany,omitempty"`
	VacancyTitle     *string            `json:"vacancyTitle,omitempty"`
	State            string             `json:"state"`
	ParentDraftID    *string            `json:"parentDraftId,omitempty"`
	Model            string             `json:"model"`
	ActivityID       *string            `json:"activityId,omitempty"`
	ExportStatus     *string            `json:"exportStatus,omitempty"`
	ExportFeedback   []ExportBlockDto   `json:"exportFeedback,omitempty"`
	ExportDocumentID *string            `json:"exportDocumentId,omitempty"`
	BaselineSummary  BaselineSummaryDto `json:"baselineSummary"`
	Proposals        []EditProposalDto  `json:"proposals"`
	CreatedAt        string             `json:"createdAt"`
	UpdatedAt        string             `json:"updatedAt"`
}

type BaselineSummaryDto struct {
	ProfileName string   `json:"profileName"`
	SkillGroups []string `json:"skillGroups"`
	Companies   []string `json:"companies"`
}

type ExportBlockDto struct {
	Field      string `json:"field"`
	Suggestion string `json:"suggestion"`
}

type TailorResumeRequestDto struct {
	ProfileID string           `json:"profileId"`
	JobID     *string          `json:"jobId,omitempty"`
	Vacancy   *AdhocVacancyDto `json:"vacancy,omitempty"`
}

type AdhocVacancyDto struct {
	Company string `json:"company"`
	Title   string `json:"title"`
	Text    string `json:"text"`
}

type ExportPdfRequestDto struct {
	DraftID string `json:"draftId"`
}
