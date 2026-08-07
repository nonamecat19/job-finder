package application

import (
	"context"

	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/generation/domain"
)

func GetDefaultMasterID(ctx context.Context, store domain.ProfileStore) (string, domain.RendercvMaster, error) {
	prof, err := store.GetDefault(ctx)
	if err != nil {
		return "", nil, err
	}
	master, err := domain.MasterFromProfile(prof)
	if err != nil {
		return "", nil, err
	}
	return dbutil.UUIDString(prof.ID), master, nil
}
