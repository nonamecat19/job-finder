package application_test

import (
	"context"
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

// fakeShape serves one configurable shape config, so a test can vary the
// resume shape without touching the settings package.
type fakeShape struct {
	cfg domain.ShapeConfig
}

func newFakeShape(cfg domain.ShapeConfig) *fakeShape { return &fakeShape{cfg: cfg} }

func (f *fakeShape) Shape(context.Context) domain.ShapeConfig { return f.cfg }

// NewService accepts the ports, not concrete values.
func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := application.NewService(&fakeRepo{}, nil, nil, nil, nil, "", "", "", nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

// A nil ShapeProvider is legal (tests and the dev fallback path); the service
// falls back to the documented defaults.
func TestNewServiceAcceptsShapeProvider(t *testing.T) {
	svc := application.NewService(&fakeRepo{}, nil, nil, nil, nil, "", "", "", newFakeShape(domain.DefaultShapeConfig()))
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
