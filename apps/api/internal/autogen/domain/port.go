package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetAutoGenerateSetting(ctx context.Context) (sqlcgen.AutoGenerateSetting, error)
	UpdateAutoGenerateSetting(ctx context.Context, arg sqlcgen.UpdateAutoGenerateSettingParams) (sqlcgen.AutoGenerateSetting, error)
}
