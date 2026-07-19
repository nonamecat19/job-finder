package ingestion_test

import (
	"testing"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/ingestion"
)

// The concrete infra types must satisfy the ports structurally.
var (
	_ ingestion.Repository = (*sqlcgen.Queries)(nil)
	_ ingestion.Enqueuer   = (*asynq.Client)(nil)
)

type fakeRepo struct {
	ingestion.Repository
}

type fakeEnqueuer struct {
	ingestion.Enqueuer
}

// The constructors accept the ports, not concrete infra values.
func TestConstructorsAcceptPorts(t *testing.T) {
	svc := ingestion.NewService(&fakeRepo{}, nil, nil, &fakeEnqueuer{})
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if ingestion.NewHandler(&fakeRepo{}, nil, nil, &fakeEnqueuer{}) == nil {
		t.Fatal("NewHandler returned nil")
	}
	if ingestion.NewScheduler(&fakeRepo{}, svc) == nil {
		t.Fatal("NewScheduler returned nil")
	}
}
