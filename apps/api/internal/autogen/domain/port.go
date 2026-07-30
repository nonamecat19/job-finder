package domain

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type Repository interface {
	GetAutoGenerateSetting(ctx context.Context, featureKey string) (sqlcgen.AiFeatureSetting, error)
	UpdateAutoGenerateSetting(ctx context.Context, arg sqlcgen.UpdateAiFeatureSettingParams) (sqlcgen.AiFeatureSetting, error)
}
