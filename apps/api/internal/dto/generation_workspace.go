package dto

type PreviewDocumentDto struct {
	Yaml string `json:"yaml"`

	SectionsHash string `json:"sectionsHash"`
}

type GenerationRunDto struct {
	ID              string               `json:"id"`
	State           string               `json:"state"`
	Vacancy         GenerationVacancyDto `json:"vacancy"`
	JobID           *string              `json:"jobId,omitempty"`
	GroundingLevel  string               `json:"groundingLevel"`
	SummaryOptionID *string              `json:"summaryOptionId,omitempty"`

	SummarySubstituted bool `json:"summarySubstituted"`

	MasterChanged bool                   `json:"masterChanged"`
	ShapeConfig   ResumeShapeConfigDto   `json:"shapeConfig"`
	Export        GenerationExportDto    `json:"export"`
	Sections      []GenerationSectionDto `json:"sections"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
}

type AdhocVacancyDto struct {
	Company string `json:"company"`
	Title   string `json:"title"`
	Text    string `json:"text"`
}

type GenerationVacancyDto struct {
	Company *string `json:"company,omitempty"`
	Title   *string `json:"title,omitempty"`
}

type GenerationSectionDto struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	EntryKey     *string `json:"entryKey,omitempty"`
	EntryLabel   *string `json:"entryLabel,omitempty"`
	Position     int     `json:"position"`
	TargetCount  int     `json:"targetCount"`
	State        string  `json:"state"`
	Error        *string `json:"error,omitempty"`
	FallbackUsed bool    `json:"fallbackUsed"`

	Enabled bool                `json:"enabled"`
	Items   []GenerationItemDto `json:"items"`
}

type GenerationItemDto struct {
	ID          string `json:"id"`
	Origin      string `json:"origin"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	SourceIndex *int   `json:"sourceIndex,omitempty"`
	Rank        int    `json:"rank"`
	Position    int    `json:"position"`
	Selected    bool   `json:"selected"`
	Edited      bool   `json:"edited"`
	Unavailable bool   `json:"unavailable"`

	SkillEntries []GenerationSkillEntryDto `json:"skillEntries,omitempty"`
}

type GenerationSkillEntryDto struct {
	Text     string `json:"text"`
	Selected bool   `json:"selected"`
}

type GenerationExportDto struct {
	Status     string             `json:"status"`
	DocumentID *string            `json:"documentId,omitempty"`
	Report     *OverflowReportDto `json:"report,omitempty"`
}

type OverflowReportDto struct {
	PagesRendered int                    `json:"pagesRendered"`
	PagesTarget   int                    `json:"pagesTarget"`
	Candidates    []OverflowCandidateDto `json:"candidates"`
}

type OverflowCandidateDto struct {
	ItemID    string `json:"itemId"`
	SectionID string `json:"sectionId"`
	Label     string `json:"label"`
	Rank      int    `json:"rank"`
}

type GenerationRewriteResponseDto struct {
	Variants []string `json:"variants"`
}

type StartGenerationRequestDto struct {
	ProfileID string           `json:"profileId"`
	JobID     *string          `json:"jobId,omitempty"`
	Vacancy   *AdhocVacancyDto `json:"vacancy,omitempty"`

	GroundingLevel *string `json:"groundingLevel,omitempty"`

	SummaryOptionID *string `json:"summaryOptionId,omitempty"`
}

type PatchGenerationItemRequestDto struct {
	Selected *bool   `json:"selected,omitempty"`
	Position *int    `json:"position,omitempty"`
	Text     *string `json:"text,omitempty"`

	DroppedEntries *[]string `json:"droppedEntries,omitempty"`
}

type PatchGenerationSectionRequestDto struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type RerunGenerationRequestDto struct {
	Sections []string `json:"sections,omitempty"`
}

type ReorderSectionItemsRequestDto struct {
	ItemIDs []string `json:"itemIds"`
}
