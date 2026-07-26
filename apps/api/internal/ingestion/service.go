package ingestion

import (
	"github.com/job-finder/api/internal/jobsources"
)

type Service struct {
	q        Repository
	registry *jobsources.Registry
	sources  *jobsources.Service
	client   Enqueuer
}

func NewService(q Repository, registry *jobsources.Registry, sources *jobsources.Service, client Enqueuer) *Service {
	return &Service{q: q, registry: registry, sources: sources, client: client}
}
