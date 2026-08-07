package application_test

import (
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/matching/application"
	"github.com/job-finder/api/internal/matching/domain"
)

var _ domain.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	domain.Repository
}

func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := application.NewService(&fakeRepo{}, nil, nil, 0.35, "")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}
