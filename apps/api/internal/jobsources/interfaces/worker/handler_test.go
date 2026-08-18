package worker_test

import (
	"context"
	"errors"
	"testing"


	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
	"github.com/job-finder/api/internal/queue"
)

func TestProcessTask_InvalidPayloadIsNotRetried(t *testing.T) {
	h := worker.NewHandler(&fakeSearchRepo{}, nil, nil, &fakeEnqueuer{})

	err := h.ProcessTask(context.Background(), queue.NewTask(queue.TypeIngest, []byte("not json")))
	if err == nil {
		t.Fatal("expected an error for an undecodable payload")
	}
	if !errors.Is(err, queue.ErrSkipRetry) {
		t.Errorf("expected the error to wrap queue.ErrSkipRetry, got: %v", err)
	}
}
