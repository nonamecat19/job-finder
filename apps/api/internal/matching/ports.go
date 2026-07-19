package matching

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the matching use-case.
// *sqlcgen.Queries satisfies it structurally. It embeds activity.Store because
// the handler resumes an activity record from the same value (activity.FromID).
type Repository interface {
	activity.Store

	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
	UpdateJobEmbedding(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingParams) error
	UpsertMatchResult(ctx context.Context, arg sqlcgen.UpsertMatchResultParams) (sqlcgen.MatchResult, error)
}
