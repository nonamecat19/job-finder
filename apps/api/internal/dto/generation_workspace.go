package dto

// PreviewDocumentDto is `GET /v1/generations/{runId}/preview-document`
// (046-real-resume-preview): the RenderCV YAML for the run's current
// selection, assembled the same way an export would but without rendering.
// The dashboard's in-browser WASM pipeline turns Yaml into a PDF preview.
type PreviewDocumentDto struct {
	Yaml string `json:"yaml"`
	// SectionsHash lets the client skip a redundant WASM re-render when two
	// edits resolve to the same effective document.
	SectionsHash string `json:"sectionsHash"`
}

// GenerationRunDto is the whole workspace: run, sections, items
// (`GET /v1/generations/{runId}`, resume-generation.md § 4.1).
type GenerationRunDto struct {
	ID              string               `json:"id"`
	State           string               `json:"state"` // running | ready | partial | failed
	Vacancy         GenerationVacancyDto `json:"vacancy"`
	JobID           *string              `json:"jobId,omitempty"`
	GroundingLevel  string               `json:"groundingLevel"`
	SummaryOptionID *string              `json:"summaryOptionId,omitempty"`
	// SummarySubstituted is the 035 provenance flag, surfaced unchanged.
	SummarySubstituted bool `json:"summarySubstituted"`
	// MasterChanged is FR-022: the run's snapshot hash no longer matches the
	// profile's current master content hash.
	MasterChanged bool                   `json:"masterChanged"`
	ShapeConfig   ResumeShapeConfigDto   `json:"shapeConfig"`
	Export        GenerationExportDto    `json:"export"`
	Sections      []GenerationSectionDto `json:"sections"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
}

// AdhocVacancyDto is a pasted, ad-hoc vacancy (no `Job` row) — moved here
// from the now-deleted dto/tailoring.go (T083): 020's tailoring surface
// never had a reader, but StartGenerationRequestDto's `vacancy` field does.
type AdhocVacancyDto struct {
	Company string `json:"company"`
	Title   string `json:"title"`
	Text    string `json:"text"`
}

// GenerationVacancyDto is the vacancy a run was made against. Text is
// intentionally omitted from the list/get response summary fields where the
// full run body isn't needed; the run detail response embeds this struct
// as-is (company/title only — vacancy_text isn't re-served, matching
// rest-api.md's example body).
type GenerationVacancyDto struct {
	Company *string `json:"company,omitempty"`
	Title   *string `json:"title,omitempty"`
}

// GenerationSectionDto is one `generation_sections` row plus its items, in
// `position` order.
type GenerationSectionDto struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"` // summary | experience | skills | projects | certifications | education
	EntryKey     *string `json:"entryKey,omitempty"`
	EntryLabel   *string `json:"entryLabel,omitempty"`
	Position     int     `json:"position"`
	TargetCount  int     `json:"targetCount"`
	State        string  `json:"state"` // running | ready | failed
	Error        *string `json:"error,omitempty"`
	FallbackUsed bool    `json:"fallbackUsed"`
	// Enabled excludes the whole section from export/preview when false,
	// independent of any item's own selection — the per-run "disable this
	// section" switch. Seeded from the account's resume-shape settings when
	// the run is created, changed afterward only via
	// `PATCH /v1/generations/{runId}/sections/{sectionId}`.
	Enabled bool                `json:"enabled"`
	Items   []GenerationItemDto `json:"items"`
}

// GenerationItemDto is one ranked candidate for inclusion — profile-sourced
// (origin="profile", text byte-identical to the master bullet at
// SourceIndex, per SC-001) or AI-suggested (origin="ai", unselected by
// default, editable, never presented as the user's material).
type GenerationItemDto struct {
	ID          string `json:"id"`
	Origin      string `json:"origin"` // profile | ai
	Kind        string `json:"kind"`   // achievement | skill_group | summary | project | certification | education
	Text        string `json:"text"`   // effective text: editedText ?? sourceText
	SourceIndex *int   `json:"sourceIndex,omitempty"`
	Rank        int    `json:"rank"`
	Position    int    `json:"position"`
	Selected    bool   `json:"selected"`
	Edited      bool   `json:"edited"`
	Unavailable bool   `json:"unavailable"`
	// SkillEntries is present only for a profile-origin item in a skills
	// section: the group's individual skills, each with its own inclusion
	// state, so the client can switch one skill off without dropping the
	// whole group. Absent everywhere else — an AI-suggested skill is a single
	// entry whose text may itself contain commas.
	SkillEntries []GenerationSkillEntryDto `json:"skillEntries,omitempty"`
}

// GenerationSkillEntryDto is one skill inside a group, and whether it is
// included in the exported resume.
type GenerationSkillEntryDto struct {
	Text     string `json:"text"`
	Selected bool   `json:"selected"`
}

// GenerationExportDto is the run's export status, embedded in the run
// response and returned as-is by `GET /v1/generations/{runId}/export`.
type GenerationExportDto struct {
	Status     string             `json:"status"` // rendering | exported | blocked | error
	DocumentID *string            `json:"documentId,omitempty"`
	Report     *OverflowReportDto `json:"report,omitempty"`
}

// OverflowReportDto is FR-019's over-budget report: pages rendered vs
// target, and the lowest-ranked selected items as named drop candidates.
// The server never acts on these.
type OverflowReportDto struct {
	PagesRendered int                    `json:"pagesRendered"`
	PagesTarget   int                    `json:"pagesTarget"`
	Candidates    []OverflowCandidateDto `json:"candidates"`
}

// OverflowCandidateDto is one named drop candidate, worst-ranked first.
type OverflowCandidateDto struct {
	ItemID    string `json:"itemId"`
	SectionID string `json:"sectionId"`
	Label     string `json:"label"`
	Rank      int    `json:"rank"`
}

// GenerationRewriteResponseDto is the body of
// `POST /v1/generations/{runId}/items/{itemId}/rewrite` — grounded alternate
// phrasings of the item's current text. Empty when the model's proposals
// didn't survive the grounding check; never an error for that case (the
// same "report, never fail" idiom the page-fit loop uses).
type GenerationRewriteResponseDto struct {
	Variants []string `json:"variants"`
}

// StartGenerationRequestDto is the body of `POST /v1/generations`. Exactly
// one of JobID / Vacancy is expected; the handler validates that, not this
// struct (rest-api.md's 400 conditions).
type StartGenerationRequestDto struct {
	ProfileID string           `json:"profileId"`
	JobID     *string          `json:"jobId,omitempty"`
	Vacancy   *AdhocVacancyDto `json:"vacancy,omitempty"`
	// GroundingLevel governs the summary only on this route (research.md
	// R1). Optional: absent means the stored default.
	GroundingLevel *string `json:"groundingLevel,omitempty"`
	// SummaryOptionID is the 034 summary-model choice for this run.
	// Optional: absent means "use my stored default".
	SummaryOptionID *string `json:"summaryOptionId,omitempty"`
}

// PatchGenerationItemRequestDto is the body of
// `PATCH /v1/generations/{runId}/items/{itemId}` — any subset of the three
// fields. Text is rejected with 403 for an origin="profile" item (FR-009 at
// the API boundary).
type PatchGenerationItemRequestDto struct {
	Selected *bool   `json:"selected,omitempty"`
	Position *int    `json:"position,omitempty"`
	Text     *string `json:"text,omitempty"`
	// DroppedEntries replaces the set of individual skills switched off
	// inside a skill group — the whole set every time, so the write is
	// idempotent and order-free. An empty (non-nil) array restores the whole
	// group. Rejected with 403 for anything but a profile-origin item in a
	// skills section, and with 400 for an entry that group does not contain.
	DroppedEntries *[]string `json:"droppedEntries,omitempty"`
}

// PatchGenerationSectionRequestDto is the body of
// `PATCH /v1/generations/{runId}/sections/{sectionId}` — currently just the
// per-run enable/disable switch, kept as its own endpoint separate from
// `.../order` since the two have unrelated validation.
type PatchGenerationSectionRequestDto struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// RerunGenerationRequestDto is the body of
// `POST /v1/generations/{runId}/rerun`. Omitting Sections reruns the whole
// run.
type RerunGenerationRequestDto struct {
	Sections []string `json:"sections,omitempty"`
}

// ReorderSectionItemsRequestDto is the body of
// `PATCH /v1/generations/{runId}/sections/{sectionId}/order` — the section's
// item ids in the caller's desired display order. The handler assigns
// `position` from each id's index in this array (T026).
type ReorderSectionItemsRequestDto struct {
	ItemIDs []string `json:"itemIds"`
}
