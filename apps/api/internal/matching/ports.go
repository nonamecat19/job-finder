package matching

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/domain"
)

type Repository interface {
	activity.Store

	GetJobByID(ctx context.Context, id pgtype.UUID) (domain.Job, error)
	UpdateJobEmbedding(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingParams) error
	UpsertMatchResult(ctx context.Context, arg sqlcgen.UpsertMatchResultParams) (sqlcgen.MatchResult, error)
}
