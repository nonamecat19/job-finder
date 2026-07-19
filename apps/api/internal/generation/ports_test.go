package generation_test

import (
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation"
)

// *sqlcgen.Queries must satisfy the Repository port structurally.
var _ generation.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	generation.Repository
}

// NewService accepts the port, not a concrete queries value.
func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := generation.NewService(&fakeRepo{}, nil, nil, nil, nil, "", "", "")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
