package retrieval

import (
	jsretrieval "github.com/nonamecat19/jobscraper/retrieval"

	"github.com/job-finder/api/internal/config"
)

// NewService builds the library's retrieval engine from the app's config. The
// ladder, the rungs and the cooling-off logic all live in the library now; the
// app contributes only the config mapping and the StateStorePort implementation.
func NewService(identity *jsretrieval.BrowserIdentity, store jsretrieval.StateStorePort, cfg *config.Config) jsretrieval.Service {
	return jsretrieval.NewEngine(identity, store, jsretrieval.EngineOpts{
		BrowserEnabled:          true,
		FlaresolverrURL:         cfg.FlaresolverrURL,
		CheapRungRetestInterval: cfg.CheapRungRetestInterval,
		CoolingOffThreshold:     cfg.CoolingOffThreshold,
		CoolingOffBaseDuration:  cfg.CoolingOffBaseDuration,
	})
}
