// Package domain holds the matching bounded context's core model: the
// Repository persistence port, the FitResult structured-LLM-output schema,
// and the ErrNoProfileConfig sentinel. Mirrors matching.service.ts.
package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// ErrNoProfileConfig is returned when the default profile has no RenderCV
// config yet, so a fit score can't be computed.
var ErrNoProfileConfig = errors.New("no profile config")

// Repository is the outbound persistence port for the matching use-case.
// *sqlcgen.Queries satisfies it structurally. It embeds activity.Store because
// the handler resumes an activity record from the same value (activity.FromID).
type Repository interface {
	activity.Store

	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpdateJobEmbedding(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingParams) error
	UpsertMatchResult(ctx context.Context, arg sqlcgen.UpsertMatchResultParams) (sqlcgen.MatchResult, error)
}

// FitResult is the structured LLM output schema, equivalent to the zod
// `fitSchema` in matching.service.ts:
//
//	z.object({
//	  score: z.number().min(0).max(100),
//	  matchedSkills: z.array(z.string()),
//	  missingSkills: z.array(z.string()),
//	  summary: z.string(),
//	  redFlags: z.array(z.string()),
//	})
type FitResult struct {
	Score         float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	MatchedSkills []string `json:"matchedSkills"`
	MissingSkills []string `json:"missingSkills"`
	Summary       string   `json:"summary"`
	RedFlags      []string `json:"redFlags"`
}

// Validate reproduces the zod .min(0).max(100) semantic check that
// completeStructured's retry loop enforces via schema.safeParse.
func (f *FitResult) Validate() error {
	if f.Score < 0 || f.Score > 100 {
		return fmt.Errorf("score must be between 0 and 100, got %v", f.Score)
	}
	return nil
}
