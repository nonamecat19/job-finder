package retrieval

import (
	"github.com/nonamecat19/job-scraper/ports"
	jsretrieval "github.com/nonamecat19/job-scraper/retrieval"
)

func ConfigureDefaultTransport(store ports.StateStore, overrides map[string]float64) {
	jsretrieval.ConfigureDefaultTransport(store, overrides)
}
