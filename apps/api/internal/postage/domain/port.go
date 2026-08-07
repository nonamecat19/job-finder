package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	PostAgeResponseRate(ctx context.Context) ([]sqlcgen.PostAgeResponseRateRow, error)
}
