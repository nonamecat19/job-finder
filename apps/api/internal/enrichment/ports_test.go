package enrichment_test

import (
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/jobsources/adapters"
)

// The concrete infra types must satisfy the ports structurally.
var (
	_ enrichment.Repository = (*sqlcgen.Queries)(nil)
	_ enrichment.Enqueuer   = (*asynq.Client)(nil)
)

type fakeRepo struct {
	enrichment.Repository
}

type fakeEnqueuer struct {
	enrichment.Enqueuer
}

// NewHandler accepts the ports, not concrete infra values.
func TestNewHandlerAcceptsPorts(t *testing.T) {
	h := enrichment.NewHandler(&fakeRepo{}, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{}, adapters.IndeedAdapter{}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, adapters.WellfoundAdapter{}, adapters.JobgetherAdapter{}, &fakeEnqueuer{}, time.Second, nil)
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
}
