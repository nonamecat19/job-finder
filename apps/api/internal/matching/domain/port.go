package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

var ErrNoProfileConfig = errors.New("no profile config")

type Repository interface {
	activity.Store

	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpdateJobEmbedding(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingParams) error
	UpsertMatchResult(ctx context.Context, arg sqlcgen.UpsertMatchResultParams) (sqlcgen.MatchResult, error)
}

type FitResult struct {
	Score         float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	MatchedSkills []string `json:"matchedSkills"`
	MissingSkills []string `json:"missingSkills"`
	Summary       string   `json:"summary"`
	RedFlags      []string `json:"redFlags"`
}

func (f *FitResult) Validate() error {
	if f.Score < 0 || f.Score > 100 {
		return fmt.Errorf("score must be between 0 and 100, got %v", f.Score)
	}
	return nil
}
