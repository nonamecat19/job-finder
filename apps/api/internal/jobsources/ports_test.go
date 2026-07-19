package jobsources_test

import (
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/jobsources"
)

// *sqlcgen.Queries must satisfy the Repository port structurally.
var _ jobsources.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	jobsources.Repository
}

// NewService accepts the port, not a concrete queries value.
func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := jobsources.NewService(&fakeRepo{}, nil, "")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
