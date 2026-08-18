package events

import (
	"encoding/json"

	"github.com/invopop/jsonschema"

	"github.com/job-finder/api/internal/queue"
)

type InputSnapshot json.RawMessage

func (s InputSnapshot) MarshalJSON() ([]byte, error) {
	if len(s) == 0 {
		return []byte("null"), nil
	}
	return s, nil
}

func (s *InputSnapshot) UnmarshalJSON(data []byte) error {
	*s = append((*s)[0:0], data...)
	return nil
}

func (InputSnapshot) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object"}
}

type MatchWork struct {
	queue.MatchPayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

type GenerateWork struct {
	queue.GeneratePayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

type SalaryWork struct {
	queue.SalaryInferPayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

type GhostWork struct {
	queue.GhostScorePayload
	Snapshot     InputSnapshot `json:"snapshot"`
	SnapshotHash string        `json:"snapshot_hash"`
}

type IngestWork struct {
	queue.IngestPayload
}

type EnrichWork struct {
	queue.EnrichPayload
}

type Usage struct {
	InputTokens  *int     `json:"input_tokens,omitempty"`
	OutputTokens *int     `json:"output_tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

type ResultStatus string

const (
	ResultSucceeded ResultStatus = "succeeded"
	ResultFailed    ResultStatus = "failed"
)

type Result struct {
	Status       ResultStatus  `json:"status"`
	Result       InputSnapshot `json:"result,omitempty"`
	Failure      *Failure      `json:"failure,omitempty"`
	TraceID      string        `json:"trace_id,omitempty"`
	SnapshotHash string        `json:"snapshot_hash,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}
