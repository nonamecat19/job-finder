package matching_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/matching"
)

type fakeRepo struct {
	matching.Repository
}

func TestNewServiceAcceptsRepositoryPort(t *testing.T) {
	svc := matching.NewService(&fakeRepo{}, nil, nil, 0.35, "")
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	_ = context.Background()
}
