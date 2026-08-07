package worker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
	"github.com/job-finder/api/internal/queue"
)

func TestProcessTask_InvalidPayloadIsNotRetried(t *testing.T) {
	h := worker.NewHandler(&fakeSearchRepo{}, nil, nil, &fakeEnqueuer{})

	err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeIngest, []byte("not json")))
	if err == nil {
		t.Fatal("expected an error for an undecodable payload")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Errorf("expected the error to wrap asynq.SkipRetry, got: %v", err)
	}
}
