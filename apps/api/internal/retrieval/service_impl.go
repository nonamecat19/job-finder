package retrieval

import (
	"github.com/nonamecat19/job-scraper/ports"
	jsretrieval "github.com/nonamecat19/job-scraper/retrieval"

	"github.com/job-finder/api/internal/config"
)

func NewService(identity *jsretrieval.BrowserIdentity, store ports.StateStore, cfg *config.Config) ports.Retriever {
	return jsretrieval.NewEngine(store,
		jsretrieval.WithIdentity(identity),
		jsretrieval.WithBrowser(true),
		jsretrieval.WithFlareSolverr(cfg.FlaresolverrURL),
		jsretrieval.WithCheapRungRetest(cfg.CheapRungRetestInterval),
		jsretrieval.WithCoolingOff(cfg.CoolingOffThreshold, cfg.CoolingOffBaseDuration),
	)
}
