package worker_test

import (
	"testing"

	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
)

type fakeSearchRepo struct {
	domain.SearchRepository
}

type fakeEnqueuer struct {
	application.Enqueuer
}

func TestConstructorsAcceptPorts(t *testing.T) {
	searchSvc := application.NewSearchService(&fakeSearchRepo{}, nil, nil, &fakeEnqueuer{})
	if worker.NewHandler(&fakeSearchRepo{}, nil, nil, &fakeEnqueuer{}) == nil {
		t.Fatal("NewHandler returned nil")
	}
	if worker.NewScheduler(&fakeSearchRepo{}, searchSvc, nil) == nil {
		t.Fatal("NewScheduler returned nil")
	}
}
