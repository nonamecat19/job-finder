package domain

import "github.com/job-finder/api/internal/dto"

type JobSource struct {
	ID      string
	Key     string
	Kind    dto.SourceKind
	Enabled bool
	Healthy bool
	Config  map[string]any
}

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
