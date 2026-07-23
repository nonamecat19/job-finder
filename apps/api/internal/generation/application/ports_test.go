package application_test

import (
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation/application"
	"github.com/job-finder/api/internal/generation/domain"
)

// *sqlcgen.Queries must satisfy the Repository port structurally.
var _ domain.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	domain.Repository
}

// NewService accepts the port, not a concrete queries value.
func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := application.NewService(&fakeRepo{}, nil, nil, nil, nil, "", "", "")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
