package domain

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	CountRepostsByDedupeKey(ctx context.Context, dedupekey string) (int32, error)
	ListJobsForCrossBoardCheck(ctx context.Context, id pgtype.UUID) ([]sqlcgen.ListJobsForCrossBoardCheckRow, error)
	CountAlwaysHiringByCompany(ctx context.Context, lower string) (int32, error)
	UpsertJobSignal(ctx context.Context, arg sqlcgen.UpsertJobSignalParams) (sqlcgen.JobSignal, error)
	GetJobSignal(ctx context.Context, arg sqlcgen.GetJobSignalParams) (sqlcgen.JobSignal, error)
}

var ErrDeclinedToScore = errors.New("ghostjob: insufficient signal to score this job")

type GhostJobResult struct {
	Score       float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	Confidence  float64  `json:"confidence" jsonschema:"minimum=0,maximum=1"`
	Explanation string   `json:"explanation"`
	TopSignals  []string `json:"topSignals"`
}

func (g *GhostJobResult) Validate() error {
	if g.Score < 0 || g.Score > 100 {
		return fmt.Errorf("score must be between 0 and 100, got %v", g.Score)
	}
	if g.Confidence < 0 || g.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1, got %v", g.Confidence)
	}
	return nil
}
