package profile

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/domain"
)

type sqlcRepository struct {
	*sqlcgen.Queries
}

func NewSqlcRepository(q *sqlcgen.Queries) Repository {
	return sqlcRepository{Queries: q}
}

func (r sqlcRepository) CreateProfile(ctx context.Context, arg sqlcgen.CreateProfileParams) (domain.Profile, error) {
	row, err := r.Queries.CreateProfile(ctx, arg)
	if err != nil {
		return domain.Profile{}, err
	}
	return profileToDomain(row), nil
}

func (r sqlcRepository) GetProfile(ctx context.Context, id pgtype.UUID) (domain.Profile, error) {
	row, err := r.Queries.GetProfile(ctx, id)
	if err != nil {
		return domain.Profile{}, err
	}
	return profileToDomain(row), nil
}

func (r sqlcRepository) GetDefaultProfile(ctx context.Context) (domain.Profile, error) {
	row, err := r.Queries.GetDefaultProfile(ctx)
	if err != nil {
		return domain.Profile{}, err
	}
	return profileToDomain(row), nil
}

func (r sqlcRepository) ListProfiles(ctx context.Context) ([]domain.Profile, error) {
	rows, err := r.Queries.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Profile, len(rows))
	for i, row := range rows {
		out[i] = profileToDomain(row)
	}
	return out, nil
}

func profileToDomain(p sqlcgen.Profile) domain.Profile {
	return domain.Profile{
		ID:             dbutil.UUIDString(p.ID),
		Name:           p.Name,
		RendercvConfig: p.RendercvConfig,
		RendercvYaml:   p.RendercvYaml,
		ExtraNotes:     p.ExtraNotes,
		CreatedAt:      p.CreatedAt.Time,
		UpdatedAt:      p.UpdatedAt.Time,
	}
}
