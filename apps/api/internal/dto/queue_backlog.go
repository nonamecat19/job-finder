package dto

// QueueBacklogDto is one queue's backlog snapshot (019-ai-job-throughput),
// assembled from the asynq Inspector plus the resolved task policy.
type QueueBacklogDto struct {
	Queue              string  `json:"queue"`
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
