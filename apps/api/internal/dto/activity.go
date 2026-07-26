package dto

type ActivityRunDto struct {
	ID         string         `json:"id"`
	Op         string         `json:"op"`
	State      string         `json:"state"`
	Label      string         `json:"label"`
	Step       *string        `json:"step"`
	JobID      *string        `json:"jobId"`
	SourceKey  *string        `json:"sourceKey"`
	RefID      *string        `json:"refId"`
	Error      *string        `json:"error"`
	Meta       map[string]any `json:"meta"`
	CreatedAt  string         `json:"createdAt"`
	StartedAt  *string        `json:"startedAt"`
	FinishedAt *string        `json:"finishedAt"`
	ElapsedMs  *int64         `json:"elapsedMs"`
}

type ActivityListResponse struct {
	Active []ActivityRunDto `json:"active"`
	Recent []ActivityRunDto `json:"recent"`
}

type FreshMatchNotificationDto struct {
	ID            string `json:"id"`
	JobId         string `json:"jobId"`
	MatchResultId string `json:"matchResultId"`
	Fresh         bool   `json:"fresh"`
	Seen          bool   `json:"seen"`
	CreatedAt     string `json:"createdAt"`
	JobTitle      *string `json:"jobTitle,omitempty"`
	Company       *string `json:"company,omitempty"`
	MatchScore    *int32  `json:"matchScore,omitempty"`
}
