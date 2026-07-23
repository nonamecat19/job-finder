// Package domain holds the ghost-job bounded context's core model: the
// Repository persistence port, the GhostSignals/GhostJobResult schemas, the
// simhash cross-board-duplicate detector, and the ErrDeclinedToScore
// sentinel. Mirrors matching/salary's domain package shape (spec 005).
package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port the ghost-job use-case depends
// on. *sqlcgen.Queries satisfies it structurally, mirroring
// matching.Repository / salary.Repository's hexagonal seam.
type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	CountRepostsByDedupeKey(ctx context.Context, dedupekey string) (int32, error)
	ListJobsForCrossBoardCheck(ctx context.Context, id pgtype.UUID) ([]sqlcgen.ListJobsForCrossBoardCheckRow, error)
	CountAlwaysHiringByCompany(ctx context.Context, lower string) (int32, error)
	UpsertJobSignal(ctx context.Context, arg sqlcgen.UpsertJobSignalParams) (sqlcgen.JobSignal, error)
	GetJobSignal(ctx context.Context, arg sqlcgen.GetJobSignalParams) (sqlcgen.JobSignal, error)
}

// ErrDeclinedToScore is returned when every signal is unknown (spec edge
// case: no postedAt, one-off company, empty description, first appearance).
// No LLM call is made and no row is written — the system declines rather
// than emitting a confident 0 or a confident 50 (SC-003).
var ErrDeclinedToScore = errors.New("ghostjob: insufficient signal to score this job")

// GhostJobResult is the structured LLM output schema, mirroring
// matching.FitResult field-for-field in shape and discipline: a plain
// struct with json + jsonschema tags plus a Validate() the retry loop in
// llm.CompleteStructured enforces (FR-010).
type GhostJobResult struct {
	Score float64 `json:"score" jsonschema:"minimum=0,maximum=100"`
	// Confidence in the score, lowered by the prompt whenever a signal is
	// unknown (FR-011); also clamped defensively by the service so a model
	// that ignores the instruction can't report false certainty (SC-005).
	Confidence float64 `json:"confidence" jsonschema:"minimum=0,maximum=1"`
	// Explanation is plain English, grounded only in the measured signals
	// handed to the prompt — never an invented fact about the employer, the
	// role, or hiring intent (FR-019).
	Explanation string `json:"explanation"`
	// TopSignals is the model's own ranking of which measured signals drove
	// the score (spec: "each contributing signal... a plain-English
	// explanation of why").
	TopSignals []string `json:"topSignals"`
}

// Validate reproduces the semantic range check llm.CompleteStructured's
// retry loop enforces via the Validator interface, exactly as
// matching.FitResult.Validate does. FR-010: a result failing this after the
// retry budget persists nothing; any prior row survives untouched.
func (g *GhostJobResult) Validate() error {
	if g.Score < 0 || g.Score > 100 {
		return fmt.Errorf("score must be between 0 and 100, got %v", g.Score)
	}
	if g.Confidence < 0 || g.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1, got %v", g.Confidence)
	}
	return nil
}
