package generation

import (
	"context"
	"encoding/json"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type ProfileStore interface {
	Get(ctx context.Context, id string) (sqlcgen.Profile, error)
	GetDefault(ctx context.Context) (sqlcgen.Profile, error)
}

func MasterFromProfile(prof sqlcgen.Profile) (RendercvMaster, error) {
	if prof.RendercvConfig == nil {
		return nil, nil
	}
	var master RendercvMaster
	if err := json.Unmarshal(prof.RendercvConfig, &master); err != nil {
		return nil, err
	}
	return master, nil
}

func GetDefaultMasterID(ctx context.Context, store ProfileStore) (string, RendercvMaster, error) {
	prof, err := store.GetDefault(ctx)
	if err != nil {
		return "", nil, err
	}
	master, err := MasterFromProfile(prof)
	if err != nil {
		return "", nil, err
	}
	return dbutil.UUIDString(prof.ID), master, nil
}
