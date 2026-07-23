package application_test

import (
	"testing"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
)

// *asynq.Client must satisfy the Enqueuer port structurally.
var _ application.Enqueuer = (*asynq.Client)(nil)

type fakeSearchRepo struct {
	domain.SearchRepository
}

type fakeEnqueuer struct {
	application.Enqueuer
}

// NewSearchService accepts the ports, not concrete infra values.
func TestNewSearchServiceAcceptsPorts(t *testing.T) {
	svc := application.NewSearchService(&fakeSearchRepo{}, nil, nil, &fakeEnqueuer{})
	if svc == nil {
		t.Fatal("NewSearchService returned nil")
	}
}
