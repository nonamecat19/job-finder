package domain

import (
	"context"
	"encoding/json"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// ProfileStore is the outbound port the generation use-case reads the
// candidate's master profile through.
type ProfileStore interface {
	Get(ctx context.Context, id string) (sqlcgen.Profile, error)
	GetDefault(ctx context.Context) (sqlcgen.Profile, error)
}

// MasterFromProfile decodes a Profile row's stored RenderCV config into a
// RendercvMaster, or (nil, nil) when the profile has none yet.
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
