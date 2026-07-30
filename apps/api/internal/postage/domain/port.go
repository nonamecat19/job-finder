package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port for the post-age signal.
// *sqlcgen.Queries satisfies it structurally.
type Repository interface {
	PostAgeResponseRate(ctx context.Context) ([]sqlcgen.PostAgeResponseRateRow, error)
}
