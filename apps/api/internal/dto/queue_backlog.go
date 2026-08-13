package dto

type QueueBacklogDto struct {
	Queue string `json:"queue"`
	// ProviderClass is permanently "hosted" for any LLM-backed queue since 044
	// removed the second (local/Ollama) inference path. The field is kept
	// rather than removed — dropping it would be a breaking change to a
	// shared DTO for a value that is now constant (plan.md Complexity
	// Tracking, contracts/configuration.md).
	ProviderClass      *string `json:"providerClass"`
	Concurrency        int     `json:"concurrency"`
	Pending            int     `json:"pending"`
	Active             int     `json:"active"`
	Scheduled          int     `json:"scheduled"`
	Retry              int     `json:"retry"`
	Archived           int     `json:"archived"`
	ProcessedPerMinute float64 `json:"processedPerMinute"`
	EtaSeconds         *int    `json:"etaSeconds"`
	Error              *string `json:"error,omitempty"`
}

type QueueBacklogResponse struct {
	Queues []QueueBacklogDto `json:"queues"`
}
