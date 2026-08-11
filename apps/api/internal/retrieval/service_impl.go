package retrieval

import (
	"github.com/nonamecat19/jobscraper/ports"
	jsretrieval "github.com/nonamecat19/jobscraper/retrieval"

	"github.com/job-finder/api/internal/config"
)

// NewService builds the library's retrieval engine from the app's config. The
// ladder, the rungs and the cooling-off logic all live in the library; the app
// contributes only the config mapping and the ports.StateStore implementation.
//
// The engine is configured through options now, so a knob the app leaves alone
// keeps the library's default rather than being zeroed by an options struct.
func NewService(identity *jsretrieval.BrowserIdentity, store ports.StateStore, cfg *config.Config) ports.Retriever {
	return jsretrieval.NewEngine(store,
		jsretrieval.WithIdentity(identity),
		jsretrieval.WithBrowser(true),
		jsretrieval.WithFlareSolverr(cfg.FlaresolverrURL),
		jsretrieval.WithCheapRungRetest(cfg.CheapRungRetestInterval),
		jsretrieval.WithCoolingOff(cfg.CoolingOffThreshold, cfg.CoolingOffBaseDuration),
	)
}
