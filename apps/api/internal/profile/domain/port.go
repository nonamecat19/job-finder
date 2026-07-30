package domain

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/domain"
)

type Repository interface {
	CreateProfile(ctx context.Context, arg sqlcgen.CreateProfileParams) (domain.Profile, error)
	DeleteProfile(ctx context.Context, id pgtype.UUID) error
	GetDefaultProfile(ctx context.Context) (domain.Profile, error)
	GetProfile(ctx context.Context, id pgtype.UUID) (domain.Profile, error)
	ListProfiles(ctx context.Context) ([]domain.Profile, error)
	ProfileHasConfig(ctx context.Context, id pgtype.UUID) (interface{}, error)
	ProfileHasEmbedding(ctx context.Context, id pgtype.UUID) (interface{}, error)
	ProfileSimilarity(ctx context.Context, arg sqlcgen.ProfileSimilarityParams) (float64, error)
	UpdateProfile(ctx context.Context, arg sqlcgen.UpdateProfileParams) error
	UpdateProfileEmbedding(ctx context.Context, arg sqlcgen.UpdateProfileEmbeddingParams) error
	UpdateProfileExtraNotes(ctx context.Context, arg sqlcgen.UpdateProfileExtraNotesParams) error
}
