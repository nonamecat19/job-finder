package domain

import "github.com/job-finder/api/internal/dto"

// JobSource is the runtime state of a single job source: its code-defined
// identity (Key/Kind, owned by the adapter registry) overlaid with the
// persisted per-source flags and its config. It is the value object the
// application service projects to dto.JobSourceDto for the API; secret config
// values are masked by the caller before projection.
type JobSource struct {
	ID      string
	Key     string
	Kind    dto.SourceKind
	Enabled bool
	Healthy bool
	Config  map[string]any
}

// ToDTO projects the entity to its API representation.
func (j JobSource) ToDTO() dto.JobSourceDto {
	return dto.JobSourceDto{
		ID:      j.ID,
		Key:     j.Key,
		Kind:    j.Kind,
		Enabled: j.Enabled,
		Healthy: j.Healthy,
		Config:  j.Config,
	}
}
