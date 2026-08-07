package domain

import (
	"context"
	"encoding/json"

	"github.com/job-finder/api/internal/db/sqlcgen"
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
