package dto

type GeneratedDocumentDto struct {
	ID        string  `json:"id"`
	JobID     *string `json:"jobId"`
	Type      string  `json:"type"`
	Version   int     `json:"version"`
	Content   any     `json:"content"`
	PdfPath   *string `json:"pdfPath"`
	Model     string  `json:"model"`
	Company   *string `json:"company,omitempty"`
	Title     *string `json:"title,omitempty"`
	Vacancy   *string `json:"vacancy,omitempty"`
	CreatedAt string  `json:"createdAt"`

	SummaryModel *string `json:"summaryModel,omitempty"`

	SummaryOptionID    *string  `json:"summaryOptionId,omitempty"`
	SummarySubstituted bool     `json:"summarySubstituted"`
	SelectionEscalated bool     `json:"selectionEscalated"`
	StageCostUsd       *float64 `json:"stageCostUsd,omitempty"`
}

type DocumentStatusDto struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Version   int    `json:"version"`
	CreatedAt string `json:"createdAt"`
}

type ApplicationDto struct {
	ID        string             `json:"id"`
	JobID     string             `json:"jobId"`
	Status    ApplicationStatus  `json:"status"`
	Notes     *string            `json:"notes"`
	AppliedAt *string            `json:"appliedAt"`
	Events    []ApplicationEvent `json:"events"`
	UpdatedAt string             `json:"updatedAt"`
	Job       *JobDto            `json:"job,omitempty"`
}

type ApplicationEvent struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

type ApplicationOutcomeDto struct {
	ID            string           `json:"id"`
	ApplicationID string           `json:"applicationId"`
	EventType     OutcomeEventType `json:"eventType"`
	OccurredAt    string           `json:"occurredAt"`
	RecordedAt    string           `json:"recordedAt"`
	Note          *string          `json:"note,omitempty"`
}

type StatsDto struct {
	JobsTotal   int64            `json:"jobsTotal"`
	JobsLast24h int64            `json:"jobsLast24h"`
	HighFit     int64            `json:"highFit"`
	Pipeline    map[string]int64 `json:"pipeline"`
	RecentRuns  []SourceRunDto   `json:"recentRuns"`
}
