package events

import (
	"encoding/json"

	"github.com/invopop/jsonschema"

	"github.com/job-finder/api/internal/queue"
)

// InputSnapshot is the grounding data an AI work event carries alongside its
// existing queue payload, because FR-008 denies the orchestration service
// database access (data-model.md § 2, E3-2). It MUST be the capability's
// complete grounding source (E3-3) and is opaque here — its shape is
// capability-specific and is not reshaped by this package.
type InputSnapshot json.RawMessage

// MarshalJSON implements json.Marshaler.
func (s InputSnapshot) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("null"), nil
	}
	return s, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *InputSnapshot) UnmarshalJSON(data []byte) error {
	*s = append((*s)[0:0], data...)
	return nil
}

// JSONSchema declares InputSnapshot to contractsgen as an arbitrary JSON
// object, rather than the base64 string reflection would otherwise infer
// from its []byte kind — a snapshot is JSON, not encoded bytes.
func (InputSnapshot) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object"}
}

// MatchWork is the payload for match.requested: the existing queue.MatchPayload
// (E3-1, unchanged) plus the input snapshot and its hash (E3-2, E3-4).
type MatchWork struct {
	queue.MatchPayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

// GenerateWork is the payload for generate.requested.
type GenerateWork struct {
	queue.GeneratePayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

// SalaryWork is the payload for salary.requested.
type SalaryWork struct {
	queue.SalaryInferPayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

// GhostWork is the payload for ghost.requested.
type GhostWork struct {
	queue.GhostScorePayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

// IngestWork is the payload for ingest.requested. Ingest stays a Go consumer
// (data-model.md § 2) and carries no snapshot — it has database access.
type IngestWork struct {
	queue.IngestPayload
}

// EnrichWork is the payload for enrich.requested. Enrich stays a Go consumer
// and carries no snapshot.
type EnrichWork struct {
	queue.EnrichPayload
}

// Usage mirrors token counts and cost from the gateway response, so a stored
// result is cheaply queryable without opening Langfuse (data-model.md § 3).
type Usage struct {
	InputTokens  *int     `json:"input_tokens,omitempty"`
	OutputTokens *int     `json:"output_tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

// ResultStatus is the closed status of a result event (E4-1).
type ResultStatus string

const (
	ResultSucceeded ResultStatus = "succeeded"
	ResultFailed    ResultStatus = "failed"
)

// Result is the payload for every *.completed event. Exactly one of Result /
// Failure is non-null, selected by Status (E4-1). The capability-specific
// result shape is opaque here — it validates against the capability's
// declared output schema (E4-2, contracts/capabilities.md).
type Result struct {
	Status       ResultStatus  `json:"status"`
	Result       InputSnapshot `json:"result,omitempty"`
	Failure      *Failure      `json:"failure,omitempty"`
	TraceID      string        `json:"trace_id,omitempty"`
	SnapshotHash string        `json:"snapshot_hash,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}
