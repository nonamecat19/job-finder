package application_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/generation/application"
	"github.com/job-finder/api/internal/generation/domain"
)

var _ domain.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	domain.Repository
}

type fakeShape struct {
	cfg domain.ShapeConfig
}

func newFakeShape(cfg domain.ShapeConfig) *fakeShape { return &fakeShape{cfg: cfg} }

func (f *fakeShape) Shape(context.Context) domain.ShapeConfig { return f.cfg }

func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := application.NewService(&fakeRepo{}, nil, nil, nil, nil, "", "", "", nil)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

func TestNewServiceAcceptsShapeProvider(t *testing.T) {
	svc := application.NewService(&fakeRepo{}, nil, nil, nil, nil, "", "", "", newFakeShape(domain.DefaultShapeConfig()))
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
