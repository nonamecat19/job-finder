package enrichment_test

import (
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/enrichment"
	djinnisrc "github.com/nonamecat19/jobscraper/adapters/djinni"
	dousrc "github.com/nonamecat19/jobscraper/adapters/dou"
	glassdoorsrc "github.com/nonamecat19/jobscraper/adapters/glassdoor"
	indeedsrc "github.com/nonamecat19/jobscraper/adapters/indeed"
	jobgethersrc "github.com/nonamecat19/jobscraper/adapters/jobgether"
	jobleadssrc "github.com/nonamecat19/jobscraper/adapters/jobleads"
	remoteoksrc "github.com/nonamecat19/jobscraper/adapters/remoteok"
	wellfoundsrc "github.com/nonamecat19/jobscraper/adapters/wellfound"
	workuasrc "github.com/nonamecat19/jobscraper/adapters/workua"
)

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

func TestNewHandlerAcceptsPorts(t *testing.T) {
	h := enrichment.NewHandler(&fakeRepo{}, nil, djinnisrc.Source{}, dousrc.Source{}, workuasrc.Source{}, indeedsrc.Source{}, remoteoksrc.Source{}, glassdoorsrc.Source{}, jobleadssrc.Source{}, wellfoundsrc.Source{}, jobgethersrc.Source{}, &fakeEnqueuer{}, time.Second, nil)
	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
}
