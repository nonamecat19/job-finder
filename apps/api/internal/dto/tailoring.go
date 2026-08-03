package dto

// EditProposalDto is one atomic, reviewable change produced by the
// tailoring proposal generator (specs/020-ai-resume-tailoring,
// data-model.md). FieldType partitions the review UI: "summary" |
// "experience_highlights" | "skill_change" | "skill_group_add" |
// "skill_group_remove". Status is "pending" | "accepted" | "rejected" |
// "dropped" ("dropped" = suppressed by grounding, never surfaced to the
// user).
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

// TraceabilityDto records where a proposed value came from: Source is
// "master" | "job_posting"; Path is a locator like
// "cv.sections.experience[Acme].highlights[2]" or "job.required_skills".
// Required on every proposal per FR-003/SC-005.
type TraceabilityDto struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

// TailoredDraftDto is a single review-lifecycle unit for tailoring a
// profile's resume against one job (or one ad-hoc vacancy). State is
// "drafting" | "proposing" | "review" | "finalized" | "expounded" |
// "abandoned". BaselineSummary is a projection of the full baseline (not
// the full RendercvMaster blob) to avoid shipping the master config to the
// client on every poll.
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

// BaselineSummaryDto is a lightweight projection of a draft's baseline:
// profile name, skill-group labels, and company list — enough for the
// review UI without shipping the full master config.
type BaselineSummaryDto struct {
	ProfileName string   `json:"profileName"`
	SkillGroups []string `json:"skillGroups"`
	Companies   []string `json:"companies"`
}

// ExportBlockDto is one actionable reason a single-page export was
// blocked, e.g. Field="experience:Acme:3",
// Suggestion="shorten or drop this bullet to fit on one page".
type ExportBlockDto struct {
	Field      string `json:"field"`
	Suggestion string `json:"suggestion"`
}

// TailorResumeRequestDto is the POST /api/tailoring request body: either
// JobID (existing job posting) or Vacancy (ad-hoc, pasted vacancy text) is
// set, not both.
type TailorResumeRequestDto struct {
	ProfileID string           `json:"profileId"`
	JobID     *string          `json:"jobId,omitempty"`
	Vacancy   *AdhocVacancyDto `json:"vacancy,omitempty"`
}

// AdhocVacancyDto is a pasted job vacancy not tracked as a Job row.
type AdhocVacancyDto struct {
	Company string `json:"company"`
	Title   string `json:"title"`
	Text    string `json:"text"`
}

// ExportPdfRequestDto is the POST /api/tailoring/{draftId}/export-pdf
// request body.
type ExportPdfRequestDto struct {
	DraftID string `json:"draftId"`
}
