package llmsettings

import (
	"context"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Repository is the outbound persistence port this package depends on.
// *sqlcgen.Queries satisfies it structurally, mirroring
// ghostjob.Repository / matching.Repository's hexagonal seam.
type Repository interface {
	ListLlmTaskSettings(ctx context.Context) ([]sqlcgen.LlmTaskSetting, error)
	UpsertLlmTaskSetting(ctx context.Context, arg sqlcgen.UpsertLlmTaskSettingParams) (sqlcgen.LlmTaskSetting, error)
}
