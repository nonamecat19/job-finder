package generation

import (
	"context"
	"encoding/json"

	"github.com/job-finder/api/internal/domain"
)

type ProfileStore interface {
	Get(ctx context.Context, id string) (domain.Profile, error)
	GetDefault(ctx context.Context) (domain.Profile, error)
}

func MasterFromConfig(rendercvConfig []byte) (RendercvMaster, error) {
	if rendercvConfig == nil {
		return nil, nil
	}
	var master RendercvMaster
	if err := json.Unmarshal(rendercvConfig, &master); err != nil {
		return nil, err
	}
	return master, nil
}

func GetDefaultMasterID(ctx context.Context, store ProfileStore) (string, RendercvMaster, error) {
	prof, err := store.GetDefault(ctx)
	if err != nil {
		return "", nil, err
	}
	master, err := MasterFromConfig(prof.RendercvConfig)
	if err != nil {
		return "", nil, err
	}
	return prof.ID, master, nil
}
