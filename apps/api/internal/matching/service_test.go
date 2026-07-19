package matching_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/matching"
)

// *sqlcgen.Queries must satisfy the Repository port structurally.
var _ matching.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	matching.Repository
}

// NewService accepts the port, not a concrete queries value.
func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := matching.NewService(&fakeRepo{}, nil, nil, 0.35, "")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	_ = context.Background()
}
